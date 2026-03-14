use std::sync::Arc;

use axum::extract::ws::{Message, WebSocket};
use futures::{SinkExt, StreamExt};
use tokio::sync::Mutex;
use tracing::{error, info, warn};

use crate::chat::scenario;
use crate::error::AppError;
use crate::grpc::context::build_context;
use crate::session::manager::{JoinResult, SessionManager};
use crate::session::types::{MessageRole, WsIncoming, WsOutgoing};
use crate::state::AppState;
use crate::webhook::types::WebhookPayload;

pub async fn handle_ws(socket: WebSocket, state: Arc<AppState>, client_ip: String) {
    let (sender, mut receiver) = socket.split();
    let sender = Arc::new(Mutex::new(sender));
    let mut session_id: Option<String> = None;

    while let Some(msg) = receiver.next().await {
        let msg = match msg {
            Ok(Message::Text(text)) => text,
            Ok(Message::Close(_)) => break,
            Ok(Message::Ping(_)) => {
                let _ = sender.lock().await.send(Message::Pong(vec![].into())).await;
                continue;
            }
            Ok(_) => continue,
            Err(e) => {
                warn!(ip = client_ip, error = %e, "websocket error");
                break;
            }
        };

        let incoming: WsIncoming = match serde_json::from_str(&msg) {
            Ok(m) => m,
            Err(e) => {
                send_error(&sender, &format!("invalid message: {e}")).await;
                continue;
            }
        };

        match incoming {
            WsIncoming::Join { name, scenario } => {
                match handle_join(&state, &sender, &client_ip, name, scenario).await {
                    Ok(id) => session_id = Some(id),
                    Err(e) => {
                        send_error(&sender, &e.to_string()).await;
                    }
                }
            }
            WsIncoming::Message { content } => {
                if let Some(ref sid) = session_id {
                    if !state.rate_limiter.check(&client_ip) {
                        send_error(&sender, "rate limited — please slow down").await;
                        continue;
                    }
                    if let Err(e) =
                        handle_message(&state, &sender, sid, &content).await
                    {
                        send_error(&sender, &e.to_string()).await;
                    }
                } else {
                    send_error(&sender, "must join first").await;
                }
            }
            WsIncoming::Resume { session_id: sid } => {
                if state.session_manager.get_session(&sid).is_some() {
                    session_id = Some(sid.clone());
                    send_json(&sender, &WsOutgoing::Session {
                        id: sid,
                        scenario: String::new(),
                    }).await;
                } else {
                    send_error(&sender, "session not found or expired").await;
                }
            }
            WsIncoming::Ping => {
                send_json(&sender, &WsOutgoing::Pong).await;
            }
        }
    }

    // Cleanup on disconnect
    if let Some(sid) = session_id {
        let (name, scenario_name) = {
            let session = state.session_manager.get_session(&sid);
            session
                .map(|s| (s.name.clone(), s.scenario.clone()))
                .unwrap_or_default()
        };

        state.session_manager.remove_session(&sid).await;
        state.webhook_dispatcher.dispatch(WebhookPayload::session_end(
            &sid, &name, &scenario_name,
        ));
        info!(session_id = sid, name = name, "session ended");
    }
}

async fn handle_join(
    state: &AppState,
    sender: &Arc<Mutex<futures::stream::SplitSink<WebSocket, Message>>>,
    client_ip: &str,
    name: String,
    scenario_name: String,
) -> Result<String, AppError> {
    // Validate name
    let sanitizer = &state.sanitizer;
    let name = sanitizer.sanitize_name(&name, state.config.session.name_max_chars)?;

    // Check scenario exists
    if state.config.get_scenario(&scenario_name).is_none() {
        return Err(AppError::InvalidMessage(format!(
            "unknown scenario: {scenario_name}"
        )));
    }

    // Rate limit check
    if !state.rate_limiter.check(client_ip) {
        return Err(AppError::RateLimited);
    }

    match state.session_manager.join(name.clone(), scenario_name.clone()).await {
        JoinResult::Admitted { session_id } => {
            info!(session_id = session_id, name = name, scenario = scenario_name, "session started");

            state.webhook_dispatcher.dispatch(WebhookPayload::session_start(
                &session_id, &name, &scenario_name,
            ));

            send_json(sender, &WsOutgoing::Session {
                id: session_id.clone(),
                scenario: scenario_name,
            }).await;

            Ok(session_id)
        }
        JoinResult::Queued { position, notify } => {
            info!(name = name, position = position, "user queued");
            send_json(sender, &WsOutgoing::Queued { position }).await;

            // Wait for admission
            notify.notified().await;

            // After being admitted, the session was created by admit_from_queue
            // We need to find it — it's the latest session with this name
            // For simplicity, we'll create a new session here
            let result = state.session_manager.join(name.clone(), scenario_name.clone()).await;
            if let JoinResult::Admitted { session_id } = result {
                send_json(sender, &WsOutgoing::Session {
                    id: session_id.clone(),
                    scenario: scenario_name,
                }).await;
                Ok(session_id)
            } else {
                Err(AppError::SessionFull)
            }
        }
        JoinResult::Full => {
            state.webhook_dispatcher.dispatch(WebhookPayload::queue_full(
                state.session_manager.queue_len().await,
                state.session_manager.active_count(),
            ));
            Err(AppError::SessionFull)
        }
    }
}

async fn handle_message(
    state: &AppState,
    sender: &Arc<Mutex<futures::stream::SplitSink<WebSocket, Message>>>,
    session_id: &str,
    content: &str,
) -> Result<(), AppError> {
    // Sanitize message
    let content = state.sanitizer.sanitize_message(content)?;

    // Get session info
    let (scenario_name, history) = {
        let mut session = state
            .session_manager
            .get_session_mut(session_id)
            .ok_or(AppError::SessionNotFound)?;

        // Add user message to history
        session.add_message(MessageRole::User, content.clone());
        session.trim_history(state.config.session.max_history_length);

        (session.scenario.clone(), session.history.clone())
    };

    // Build RAG context
    let collection = scenario::get_collection(&state.config, &scenario_name);
    let context = {
        let mut client = state.mddb_client.clone();
        build_context(&mut client, &content, &collection).await.unwrap_or_default()
    };

    // Build messages for LLM
    let messages = scenario::build_messages(&state.config, &scenario_name, &context, &history);

    let temperature = scenario::get_temperature(&state.config, &scenario_name);
    let max_tokens = state.config.llm.max_tokens;

    // Stream LLM response
    let mut stream = state.llm_provider.chat_stream(&messages, temperature, max_tokens).await?;
    let mut full_response = String::new();

    while let Some(chunk) = stream.next().await {
        match chunk {
            Ok(text) => {
                full_response.push_str(&text);

                // Check response length limit
                if full_response.len() > state.config.session.max_response_length {
                    send_json(sender, &WsOutgoing::Chunk {
                        content: "\n\n[Response truncated]".to_string(),
                    }).await;
                    break;
                }

                send_json(sender, &WsOutgoing::Chunk { content: text }).await;
            }
            Err(e) => {
                error!(error = %e, "LLM stream error");
                send_error(sender, "error generating response").await;
                break;
            }
        }
    }

    send_json(sender, &WsOutgoing::Done).await;

    // Save assistant response to history
    if let Some(mut session) = state.session_manager.get_session_mut(session_id) {
        session.add_message(MessageRole::Assistant, full_response);
    }

    Ok(())
}

async fn send_json<T: serde::Serialize>(
    sender: &Arc<Mutex<futures::stream::SplitSink<WebSocket, Message>>>,
    msg: &T,
) {
    if let Ok(json) = serde_json::to_string(msg) {
        let _ = sender.lock().await.send(Message::Text(json.into())).await;
    }
}

async fn send_error(
    sender: &Arc<Mutex<futures::stream::SplitSink<WebSocket, Message>>>,
    message: &str,
) {
    send_json(sender, &WsOutgoing::Error {
        message: message.to_string(),
    }).await;
}
