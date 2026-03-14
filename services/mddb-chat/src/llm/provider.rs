use futures::Stream;
use std::pin::Pin;

use crate::session::types::ChatMessage;

pub type ChunkStream = Pin<Box<dyn Stream<Item = Result<String, crate::error::AppError>> + Send>>;

#[async_trait::async_trait]
pub trait LlmProvider: Send + Sync {
    /// Send messages and get a streaming response
    async fn chat_stream(
        &self,
        messages: &[ChatMessage],
        temperature: f32,
        max_tokens: u32,
    ) -> Result<ChunkStream, crate::error::AppError>;
}
