use std::sync::Arc;

use axum::extract::State;
use axum::Json;
use serde_json::{json, Value};

use crate::state::AppState;

pub async fn health(State(state): State<Arc<AppState>>) -> Json<Value> {
    Json(json!({
        "status": "ok",
        "service": "mddb-chat",
        "active_sessions": state.session_manager.active_count(),
        "queue_length": state.session_manager.queue_len().await,
        "max_concurrent": state.config.session.max_concurrent,
    }))
}

pub async fn config_info(State(state): State<Arc<AppState>>) -> Json<Value> {
    let scenarios: Vec<&str> = state.config.scenarios.keys().map(|k| k.as_str()).collect();
    Json(json!({
        "scenarios": scenarios,
        "max_concurrent": state.config.session.max_concurrent,
        "queue_size": state.config.session.queue_size,
        "max_message_length": state.config.security.max_message_length,
        "name_max_chars": state.config.session.name_max_chars,
    }))
}
