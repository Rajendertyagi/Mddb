#!/usr/bin/env python3
"""
MDDB Python Client Example

Demonstrates basic operations and vector (semantic) search.
"""

from mddb import MDDB


def main():
    # Connect in write mode
    db = MDDB.connect('localhost:11023', 'write')
    db = db.collection('docs')

    # Add documents
    print("Adding documents...")
    db.add('auth-guide', 'en_US',
           {'category': ['security'], 'type': ['guide']},
           '# Authentication Guide\n\nHow to authenticate users with JWT tokens.')

    db.add('deploy-guide', 'en_US',
           {'category': ['devops'], 'type': ['guide']},
           '# Deployment Guide\n\nDeploy to production with Docker and Kubernetes.')

    print("Documents added.\n")

    # Metadata search
    print("Metadata search (category=security):")
    results = db.search('category', 'security')
    for doc in results:
        print(f"  - {doc['key']} ({doc['lang']})")

    # Vector search - find by meaning
    print("\nVector search: 'how to login users'")
    resp = db.vector_search('how to login users', top_k=5, include_content=False)

    if resp.get('results'):
        for r in resp['results']:
            score_pct = round(r['score'] * 100)
            print(f"  #{r['rank']}  {score_pct}%  {r['document']['key']}")
        print(f"  Model: {resp.get('model')}, Dims: {resp.get('dimensions')}")
    else:
        print("  No results (is embedding provider configured?)")

    # Vector stats
    print("\nVector stats:")
    vs = db.vector_stats()
    print(f"  Enabled: {vs.get('enabled')}")
    if vs.get('enabled'):
        print(f"  Model: {vs.get('model')}")
        for name, info in vs.get('collections', {}).items():
            print(f"  {name}: {info['embedded_documents']}/{info['total_documents']} embedded")

    # Server stats
    print("\nServer stats:")
    stats = db.stats()
    print(f"  DB size: {stats['databaseSize'] / 1024:.1f} KB")
    print(f"  Total docs: {stats['totalDocuments']}")


if __name__ == '__main__':
    try:
        main()
    except RuntimeError as e:
        print(f"Error: {e}")
    except Exception as e:
        print(f"Error: {e}")
