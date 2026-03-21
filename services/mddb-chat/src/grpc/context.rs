use crate::error::AppError;
use crate::grpc::client::MddbClient;

/// Build RAG context from MDDB search results
pub async fn build_context(
    client: &mut MddbClient,
    query: &str,
    collection: &str,
) -> Result<String, AppError> {
    let results = client.search(query, collection).await?;

    if results.is_empty() {
        return Ok(String::new());
    }

    let mut context = String::from("## Relevant Documentation\n\n");

    for (i, result) in results.iter().enumerate() {
        context.push_str(&format!(
            "### Source {} (score: {:.2})\n**Key:** {}\n\n{}\n\n---\n\n",
            i + 1,
            result.score,
            result.key,
            result.content
        ));
    }

    Ok(context)
}
