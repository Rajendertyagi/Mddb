use serde::{Deserialize, Serialize};
use std::time::Instant;

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ChatMessage {
    pub role: MessageRole,
    pub content: String,
    pub timestamp: u64,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
#[serde(rename_all = "lowercase")]
pub enum MessageRole {
    User,
    Assistant,
    System,
}

#[derive(Debug)]
pub struct Session {
    pub id: String,
    pub name: String,
    pub scenario: String,
    pub history: Vec<ChatMessage>,
    pub created_at: Instant,
    pub last_active: Instant,
    pub message_count: usize,
    pub total_tokens_used: usize,
}

impl Session {
    pub fn new(id: String, name: String, scenario: String) -> Self {
        let now = Instant::now();
        Self {
            id,
            name,
            scenario,
            history: Vec::new(),
            created_at: now,
            last_active: now,
            message_count: 0,
            total_tokens_used: 0,
        }
    }

    pub fn add_message(&mut self, role: MessageRole, content: String) {
        let timestamp = std::time::SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)
            .unwrap_or_default()
            .as_secs();

        self.history.push(ChatMessage {
            role,
            content,
            timestamp,
        });
        self.last_active = Instant::now();
        self.message_count += 1;
    }

    pub fn trim_history(&mut self, max_length: usize) {
        if self.history.len() > max_length {
            let drain_count = self.history.len() - max_length;
            self.history.drain(..drain_count);
        }
    }
}

/// Messages sent over WebSocket
#[derive(Debug, Serialize, Deserialize)]
#[serde(tag = "type", rename_all = "lowercase")]
pub enum WsIncoming {
    Join {
        name: String,
        #[serde(default = "default_scenario")]
        scenario: String,
    },
    Message {
        content: String,
    },
    Resume {
        session_id: String,
    },
    End,
    Feedback {
        rating: String,
        question: String,
        answer: String,
    },
    Ping,
}

#[derive(Debug, Serialize, Deserialize)]
#[serde(tag = "type", rename_all = "lowercase")]
pub enum WsOutgoing {
    Session {
        id: String,
        scenario: String,
    },
    Queued {
        position: usize,
    },
    Chunk {
        content: String,
    },
    Done,
    Error {
        message: String,
    },
    Pong,
    Ended,
}

fn default_scenario() -> String {
    "assistant".to_string()
}
