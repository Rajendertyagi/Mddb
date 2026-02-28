#!/usr/bin/env python3
"""
MDDB Python Client Example

This example demonstrates how to use the MDDB gRPC client in Python.
"""

import grpc
from mddb_client import mddb_pb2, mddb_pb2_grpc


def main():
    # Connect to MDDB server
    channel = grpc.insecure_channel('localhost:11024')
    client = mddb_pb2_grpc.MDDBStub(channel)

    print("🔗 Connected to MDDB server")
    print()

    # ========================================================================
    # Add a document
    # ========================================================================
    print("📝 Adding a document...")
    doc = client.Add(mddb_pb2.AddRequest(
        collection='blog',
        key='python-example',
        lang='en_US',
        meta={
            'category': mddb_pb2.MetaValues(values=['tutorial', 'python']),
            'author': mddb_pb2.MetaValues(values=['Python Developer']),
            'tags': mddb_pb2.MetaValues(values=['grpc', 'api', 'example'])
        },
        content_md='# Python gRPC Example\n\nThis document was created using the Python client!'
    ))
    print(f"✅ Document added: {doc.id}")
    print(f"   Added at: {doc.added_at}")
    print()

    # ========================================================================
    # Get the document
    # ========================================================================
    print("📖 Retrieving document...")
    doc = client.Get(mddb_pb2.GetRequest(
        collection='blog',
        key='python-example',
        lang='en_US',
        env={'year': '2024', 'language': 'Python'}
    ))
    print(f"✅ Retrieved: {doc.key}")
    print(f"   Content: {doc.content_md[:50]}...")
    print()

    # ========================================================================
    # Search documents
    # ========================================================================
    print("🔍 Searching for tutorial documents...")
    resp = client.Search(mddb_pb2.SearchRequest(
        collection='blog',
        filter_meta={
            'category': mddb_pb2.MetaValues(values=['tutorial'])
        },
        sort='updatedAt',
        asc=False,
        limit=10
    ))
    print(f"✅ Found {len(resp.documents)} documents")
    for i, doc in enumerate(resp.documents, 1):
        print(f"   {i}. {doc.key} ({doc.lang})")
    print()

    # ========================================================================
    # Get server statistics
    # ========================================================================
    print("📊 Getting server statistics...")
    stats = client.Stats(mddb_pb2.StatsRequest())
    print(f"✅ Server Stats:")
    print(f"   Database: {stats.database_path}")
    print(f"   Size: {stats.database_size / 1024:.2f} KB")
    print(f"   Mode: {stats.mode}")
    print(f"   Total Documents: {stats.total_documents}")
    print(f"   Total Revisions: {stats.total_revisions}")
    print()

    if stats.collections:
        print("   Collections:")
        for coll in stats.collections:
            print(f"     • {coll.name}: {coll.document_count} docs, "
                  f"{coll.revision_count} revisions")
    print()

    # ========================================================================
    # Create backup
    # ========================================================================
    print("💾 Creating backup...")
    backup_resp = client.Backup(mddb_pb2.BackupRequest(
        to='python-backup.db'
    ))
    print(f"✅ Backup created: {backup_resp.backup}")
    print()

    # ========================================================================
    # Vector Search (semantic search)
    # ========================================================================
    print("🧠 Vector search: 'how to use the API'...")
    try:
        resp = client.VectorSearch(mddb_pb2.VectorSearchRequest(
            collection='blog',
            query='how to use the API',
            top_k=5,
            include_content=False
        ))
        print(f"✅ Found {len(resp.results)} similar documents")
        for r in resp.results:
            print(f"   #{r.rank}  {r.score:.0%}  {r.document.key} ({r.document.lang})")
        if resp.model:
            print(f"   Model: {resp.model}, Dimensions: {resp.dimensions}")
    except grpc.RpcError as e:
        print(f"   ⚠️  Vector search unavailable: {e.details()}")
    print()

    # ========================================================================
    # Vector Stats
    # ========================================================================
    print("📊 Vector stats...")
    try:
        vs = client.VectorStats(mddb_pb2.VectorStatsRequest())
        print(f"✅ Embeddings: {'enabled' if vs.enabled else 'disabled'}")
        if vs.enabled:
            print(f"   Model: {vs.model}, Dimensions: {vs.dimensions}")
            for name, cs in vs.collections.items():
                print(f"   {name}: {cs.embedded_documents}/{cs.total_documents} embedded")
    except grpc.RpcError as e:
        print(f"   ⚠️  Vector stats unavailable: {e.details()}")
    print()

    print("✨ All operations completed successfully!")
    channel.close()


if __name__ == '__main__':
    try:
        main()
    except grpc.RpcError as e:
        print(f"❌ gRPC Error: {e.code()} - {e.details()}")
    except Exception as e:
        print(f"❌ Error: {e}")
