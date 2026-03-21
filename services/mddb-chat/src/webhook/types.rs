use serde::Serialize;

#[derive(Debug, Clone, Serialize)]
pub struct WebhookPayload {
    pub event: String,
    pub timestamp: u64,
    pub data: WebhookData,
}

#[derive(Debug, Clone, Serialize)]
#[serde(untagged)]
pub enum WebhookData {
    Session {
        session_id: String,
        user_name: String,
        scenario: String,
    },
    Message {
        session_id: String,
        user_name: String,
        content: String,
    },
    Queue {
        queue_size: usize,
        active_sessions: usize,
    },
}

impl WebhookPayload {
    pub fn session_start(session_id: &str, user_name: &str, scenario: &str) -> Self {
        Self {
            event: "session.start".to_string(),
            timestamp: now_unix(),
            data: WebhookData::Session {
                session_id: session_id.to_string(),
                user_name: user_name.to_string(),
                scenario: scenario.to_string(),
            },
        }
    }

    pub fn session_end(session_id: &str, user_name: &str, scenario: &str) -> Self {
        Self {
            event: "session.end".to_string(),
            timestamp: now_unix(),
            data: WebhookData::Session {
                session_id: session_id.to_string(),
                user_name: user_name.to_string(),
                scenario: scenario.to_string(),
            },
        }
    }

    pub fn queue_full(queue_size: usize, active_sessions: usize) -> Self {
        Self {
            event: "queue.full".to_string(),
            timestamp: now_unix(),
            data: WebhookData::Queue {
                queue_size,
                active_sessions,
            },
        }
    }
}

fn now_unix() -> u64 {
    std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .unwrap_or_default()
        .as_secs()
}
