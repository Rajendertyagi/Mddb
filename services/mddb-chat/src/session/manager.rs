use std::collections::VecDeque;
use std::sync::Arc;
use std::time::Duration;

use dashmap::DashMap;
use tokio::sync::{Mutex, Notify};
use uuid::Uuid;

use crate::config::SessionConfig;
use crate::session::types::Session;

pub struct SessionManager {
    active: DashMap<String, Session>,
    queue: Mutex<VecDeque<QueueEntry>>,
    notify: Notify,
    config: SessionConfig,
}

struct QueueEntry {
    pub name: String,
    pub scenario: String,
    pub notify: Arc<Notify>,
}

pub enum JoinResult {
    Admitted { session_id: String },
    Queued { position: usize, notify: Arc<Notify> },
    Full,
}

impl SessionManager {
    pub fn new(config: SessionConfig) -> Self {
        Self {
            active: DashMap::new(),
            queue: Mutex::new(VecDeque::new()),
            notify: Notify::new(),
            config,
        }
    }

    pub async fn join(&self, name: String, scenario: String) -> JoinResult {
        // Check if there's room
        if self.active.len() < self.config.max_concurrent {
            let session_id = Uuid::new_v4().to_string();
            let session = Session::new(session_id.clone(), name, scenario);
            self.active.insert(session_id.clone(), session);
            return JoinResult::Admitted { session_id };
        }

        // Try to queue
        let mut queue = self.queue.lock().await;
        if queue.len() >= self.config.queue_size {
            return JoinResult::Full;
        }

        let notify = Arc::new(Notify::new());
        queue.push_back(QueueEntry {
            name,
            scenario,
            notify: notify.clone(),
        });
        let position = queue.len();
        JoinResult::Queued { position, notify }
    }

    /// Called when a queued user is notified — creates their session
    pub async fn admit_from_queue(&self) -> Option<(String, String, String)> {
        let mut queue = self.queue.lock().await;
        if let Some(entry) = queue.pop_front() {
            let session_id = Uuid::new_v4().to_string();
            let session = Session::new(session_id.clone(), entry.name.clone(), entry.scenario.clone());
            self.active.insert(session_id.clone(), session);
            entry.notify.notify_one();
            Some((session_id, entry.name, entry.scenario))
        } else {
            None
        }
    }

    pub fn get_session(&self, id: &str) -> Option<dashmap::mapref::one::Ref<'_, String, Session>> {
        self.active.get(id)
    }

    pub fn get_session_mut(&self, id: &str) -> Option<dashmap::mapref::one::RefMut<'_, String, Session>> {
        self.active.get_mut(id)
    }

    pub async fn remove_session(&self, id: &str) {
        self.active.remove(id);
        // Try to admit next from queue
        self.admit_from_queue().await;
    }

    pub fn active_count(&self) -> usize {
        self.active.len()
    }

    pub async fn queue_len(&self) -> usize {
        self.queue.lock().await.len()
    }

    /// Clean up expired sessions
    pub async fn cleanup_expired(&self) {
        let ttl = Duration::from_secs(self.config.session_ttl_minutes * 60);
        let expired: Vec<String> = self
            .active
            .iter()
            .filter(|entry| entry.last_active.elapsed() > ttl)
            .map(|entry| entry.key().clone())
            .collect();

        for id in expired {
            self.active.remove(&id);
        }

        // Try to admit queued users for freed slots
        while self.active.len() < self.config.max_concurrent {
            if self.admit_from_queue().await.is_none() {
                break;
            }
        }
    }
}
