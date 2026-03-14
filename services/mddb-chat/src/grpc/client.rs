use tonic::transport::Channel;

use crate::config::MddbConfig;
use crate::error::AppError;

pub mod proto {
    tonic::include_proto!("mddb");
}

#[derive(Clone)]
pub struct MddbClient {
    client: proto::mddb_client::MddbClient<Channel>,
    config: MddbConfig,
}

impl MddbClient {
    pub async fn connect(config: MddbConfig) -> Result<Self, AppError> {
        let client = proto::mddb_client::MddbClient::connect(config.grpc_addr.clone())
            .await
            .map_err(|e| AppError::Internal(format!("failed to connect to mddbd: {e}")))?;

        Ok(Self { client, config })
    }

    /// Search documents using hybrid search (FTS + vector)
    pub async fn hybrid_search(
        &mut self,
        query: &str,
        collection: &str,
        top_k: u32,
    ) -> Result<Vec<SearchResult>, AppError> {
        let request = proto::HybridSearchRequest {
            query: query.to_string(),
            collection: collection.to_string(),
            limit: top_k as i32,
            ..Default::default()
        };

        let response = self
            .client
            .hybrid_search(request)
            .await
            .map_err(AppError::GrpcError)?;

        let results = response
            .into_inner()
            .results
            .into_iter()
            .filter_map(|r| {
                let doc = r.document?;
                Some(SearchResult {
                    key: doc.key,
                    content: doc.content_md,
                    score: r.combined_score as f32,
                    collection: collection.to_string(),
                })
            })
            .collect();

        Ok(results)
    }

    /// Search documents using full-text search
    pub async fn fts_search(
        &mut self,
        query: &str,
        collection: &str,
        top_k: u32,
    ) -> Result<Vec<SearchResult>, AppError> {
        let request = proto::FtsRequest {
            query: query.to_string(),
            collection: collection.to_string(),
            limit: top_k as i32,
            ..Default::default()
        };

        let response = self
            .client
            .fts(request)
            .await
            .map_err(AppError::GrpcError)?;

        let results = response
            .into_inner()
            .results
            .into_iter()
            .filter_map(|r| {
                let doc = r.document?;
                Some(SearchResult {
                    key: doc.key,
                    content: doc.content_md,
                    score: r.score as f32,
                    collection: collection.to_string(),
                })
            })
            .collect();

        Ok(results)
    }

    /// Search documents using vector/semantic search
    pub async fn vector_search(
        &mut self,
        query: &str,
        collection: &str,
        top_k: u32,
    ) -> Result<Vec<SearchResult>, AppError> {
        let request = proto::VectorSearchRequest {
            query: query.to_string(),
            collection: collection.to_string(),
            top_k: top_k as i32,
            ..Default::default()
        };

        let response = self
            .client
            .vector_search(request)
            .await
            .map_err(AppError::GrpcError)?;

        let results = response
            .into_inner()
            .results
            .into_iter()
            .filter_map(|r| {
                let doc = r.document?;
                Some(SearchResult {
                    key: doc.key,
                    content: doc.content_md,
                    score: r.score,
                    collection: collection.to_string(),
                })
            })
            .collect();

        Ok(results)
    }

    /// Search using the configured search type
    pub async fn search(
        &mut self,
        query: &str,
        collection: &str,
    ) -> Result<Vec<SearchResult>, AppError> {
        let top_k = self.config.search_top_k;
        match self.config.search_type.as_str() {
            "hybrid" => self.hybrid_search(query, collection, top_k).await,
            "vector" => self.vector_search(query, collection, top_k).await,
            "fts" => self.fts_search(query, collection, top_k).await,
            other => Err(AppError::Internal(format!("unknown search type: {other}"))),
        }
    }
}

#[derive(Debug, Clone)]
pub struct SearchResult {
    pub key: String,
    pub content: String,
    pub score: f32,
    pub collection: String,
}
