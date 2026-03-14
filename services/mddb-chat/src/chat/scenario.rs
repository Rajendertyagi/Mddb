use crate::config::Config;
use crate::session::types::{ChatMessage, MessageRole};

/// Build the full message list for the LLM call
pub fn build_messages(
    config: &Config,
    scenario_name: &str,
    context: &str,
    history: &[ChatMessage],
) -> Vec<ChatMessage> {
    let mut messages = Vec::new();

    // System prompt from scenario
    let system_prompt = if let Some(scenario) = config.get_scenario(scenario_name) {
        &scenario.system_prompt
    } else {
        "You are a helpful assistant. Answer questions based on the provided context."
    };

    let mut system_content = system_prompt.to_string();

    // Append RAG context if available
    if !context.is_empty() {
        system_content.push_str("\n\n");
        system_content.push_str("Use the following documentation to answer the user's question. ");
        system_content.push_str("If the answer is not in the documentation, say so honestly.\n\n");
        system_content.push_str(context);
    }

    messages.push(ChatMessage {
        role: MessageRole::System,
        content: system_content,
        timestamp: 0,
    });

    // Conversation history
    messages.extend_from_slice(history);

    messages
}

/// Get the temperature for a scenario (or default)
pub fn get_temperature(config: &Config, scenario_name: &str) -> f32 {
    config
        .get_scenario(scenario_name)
        .and_then(|s| s.temperature)
        .unwrap_or(config.llm.temperature)
}

/// Get the collection to search for a scenario
pub fn get_collection(config: &Config, scenario_name: &str) -> String {
    config
        .get_scenario(scenario_name)
        .and_then(|s| s.allowed_collections.first().cloned())
        .unwrap_or_else(|| config.mddb.default_collection.clone())
}
