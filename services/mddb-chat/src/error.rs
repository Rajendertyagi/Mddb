use axum::http::StatusCode;
use axum::response::{IntoResponse, Response};
use serde_json::json;

#[derive(Debug)]
pub enum AppError {
    SessionFull,
    SessionNotFound,
    InvalidMessage(String),
    RateLimited,
    MessageTooLong,
    NameTooLong,
    GrpcError(tonic::Status),
    LlmError(String),
    WebhookError(String),
    Internal(String),
}

impl std::fmt::Display for AppError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            AppError::SessionFull => write!(f, "session queue is full"),
            AppError::SessionNotFound => write!(f, "session not found"),
            AppError::InvalidMessage(msg) => write!(f, "invalid message: {msg}"),
            AppError::RateLimited => write!(f, "rate limited"),
            AppError::MessageTooLong => write!(f, "message too long"),
            AppError::NameTooLong => write!(f, "name too long"),
            AppError::GrpcError(s) => write!(f, "grpc error: {s}"),
            AppError::LlmError(msg) => write!(f, "llm error: {msg}"),
            AppError::WebhookError(msg) => write!(f, "webhook error: {msg}"),
            AppError::Internal(msg) => write!(f, "internal error: {msg}"),
        }
    }
}

impl std::error::Error for AppError {}

impl IntoResponse for AppError {
    fn into_response(self) -> Response {
        let (status, message) = match &self {
            AppError::SessionFull => (StatusCode::SERVICE_UNAVAILABLE, self.to_string()),
            AppError::SessionNotFound => (StatusCode::NOT_FOUND, self.to_string()),
            AppError::InvalidMessage(_) => (StatusCode::BAD_REQUEST, self.to_string()),
            AppError::RateLimited => (StatusCode::TOO_MANY_REQUESTS, self.to_string()),
            AppError::MessageTooLong => (StatusCode::BAD_REQUEST, self.to_string()),
            AppError::NameTooLong => (StatusCode::BAD_REQUEST, self.to_string()),
            AppError::GrpcError(_) => (StatusCode::BAD_GATEWAY, self.to_string()),
            AppError::LlmError(_) => (StatusCode::BAD_GATEWAY, self.to_string()),
            AppError::WebhookError(_) => (StatusCode::INTERNAL_SERVER_ERROR, self.to_string()),
            AppError::Internal(_) => (StatusCode::INTERNAL_SERVER_ERROR, self.to_string()),
        };

        let body = axum::Json(json!({ "error": message }));
        (status, body).into_response()
    }
}

impl From<tonic::Status> for AppError {
    fn from(status: tonic::Status) -> Self {
        AppError::GrpcError(status)
    }
}
