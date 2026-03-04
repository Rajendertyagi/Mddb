import { useEffect, useState } from 'react';
import { Code, Copy, Download, Check, Brain } from 'lucide-react';
import { useStore } from '../lib/store';
import mddbClient from '../lib/mddb-client';

const TABS = [
  { id: 'mcp', label: 'MCP Config', desc: 'Custom MCP tools configuration (YAML)' },
  { id: 'claude', label: 'Claude Desktop', desc: 'Anthropic Claude Desktop / Claude Code' },
  { id: 'chatgpt', label: 'ChatGPT', desc: 'OpenAI ChatGPT with MCP bridge' },
  { id: 'ollama', label: 'Ollama (Python)', desc: 'Local Ollama with Python MCP client' },
  { id: 'deepseek', label: 'DeepSeek', desc: 'DeepSeek AI agent connection' },
  { id: 'manus', label: 'Manus', desc: 'Manus AI agent configuration' },
  { id: 'bielik', label: 'Bielik.ai', desc: 'Bielik.ai Polish LLM agent' },
  { id: 'openwebui', label: 'Open WebUI', desc: 'Open WebUI RAG pipeline model' },
];

function generateConfig(tabId, grpcAddr, httpAddr) {
  const grpc = grpcAddr || 'localhost:11024';
  const http = httpAddr || 'http://localhost:11023';
  const grpcHost = grpc.startsWith(':') ? `localhost${grpc}` : grpc;
  const httpBase = http.startsWith(':') ? `http://localhost${http}` : http;

  switch (tabId) {
    case 'mcp':
      return {
        filename: 'mcp_config.yaml',
        content: `# MDDB MCP Custom Tools Configuration
# Set MDDB_MCP_CONFIG=/path/to/this/file.yaml to load

# Custom tools wrap built-in tools with preset defaults
# They appear alongside built-in tools in the MCP tool list
custom_tools:
  - name: "kb_search"
    description: "Search the knowledge base using semantic similarity"
    action: "semantic_search"
    defaults:
      collection: "docs"
      top_k: 5
      threshold: 0.7
      includeContent: true
    parameters:
      - name: "query"
        type: "string"
        description: "Search query"
        required: true

  - name: "kb_lookup"
    description: "Full-text keyword search in the knowledge base"
    action: "full_text_search"
    defaults:
      collection: "docs"
      limit: 10
      algorithm: "bm25"
    parameters:
      - name: "query"
        type: "string"
        description: "Search keywords"
        required: true`,
        instructions: [
          'MCP is now built into the MDDB server — no separate service needed',
          'Custom tools are optional — save this file and set MDDB_MCP_CONFIG env var',
          'For stdio mode (Claude Desktop): set MDDB_MCP_STDIO=true',
          'For HTTP mode: MCP endpoints are always available at /mcp/*',
        ],
      };

    case 'claude':
      return {
        filename: 'claude_desktop_config.json',
        content: JSON.stringify({
          mcpServers: {
            mddb: {
              command: 'docker',
              args: [
                'run', '-i', '--rm', '--network', 'host',
                '-v', 'mddb-data:/app/data',
                '-e', 'MDDB_MCP_STDIO=true',
                'tradik/mddb:latest',
              ],
              env: {},
            },
          },
        }, null, 2),
        instructions: [
          'Copy this configuration to your Claude Desktop config file:',
          'macOS: ~/Library/Application Support/Claude/claude_desktop_config.json',
          'Windows: %APPDATA%\\Claude\\claude_desktop_config.json',
          'Linux: ~/.config/Claude/claude_desktop_config.json',
          'Restart Claude Desktop to apply changes',
        ],
        alt: {
          label: 'Local Binary (no Docker)',
          content: JSON.stringify({
            mcpServers: {
              mddb: {
                command: '/path/to/mddbd',
                args: [],
                env: {
                  MDDB_MCP_STDIO: 'true',
                  MDDB_PATH: '/path/to/mddb.db',
                },
              },
            },
          }, null, 2),
        },
      };

    case 'chatgpt':
      return {
        filename: 'openai_actions.json',
        content: JSON.stringify({
          openapi: '3.1.0',
          info: { title: 'MDDB API', version: '2.6.0', description: 'Markdown Database API for ChatGPT' },
          servers: [{ url: httpBase }],
          paths: {
            '/v1/search': {
              post: {
                operationId: 'searchDocuments',
                summary: 'Search documents by metadata',
                requestBody: {
                  required: true,
                  content: { 'application/json': { schema: { type: 'object', properties: {
                    collection: { type: 'string' },
                    filterMeta: { type: 'object' },
                    sort: { type: 'string' },
                    limit: { type: 'integer', default: 10 },
                  }}}},
                },
              },
            },
            '/v1/vector/search': {
              post: {
                operationId: 'semanticSearch',
                summary: 'Semantic search using vector embeddings',
                requestBody: {
                  required: true,
                  content: { 'application/json': { schema: { type: 'object', properties: {
                    collection: { type: 'string' },
                    query: { type: 'string' },
                    topK: { type: 'integer', default: 5 },
                    threshold: { type: 'number', default: 0.7 },
                  }}}},
                },
              },
            },
          },
        }, null, 2),
        instructions: [
          'Go to https://platform.openai.com/gpts and create or edit a Custom GPT',
          'In "Configure" tab, scroll to "Actions" and click "Create new action"',
          'Paste this OpenAPI schema into the schema editor',
          `Make sure your MDDB server at ${httpBase} is publicly accessible`,
          'Save and test the GPT with prompts like "Search my docs for..."',
        ],
      };

    case 'ollama':
      return {
        filename: 'mddb_ollama_agent.py',
        content: `#!/usr/bin/env python3
"""MDDB + Ollama RAG Agent

Uses Ollama for LLM inference and MDDB for document retrieval.
Supports semantic search, full-text search, and metadata queries.

Requirements:
  pip install requests ollama
"""

import requests
import ollama

MDDB_URL = "${httpBase}"
OLLAMA_MODEL = "llama3.2"  # or any model you have pulled

def search_mddb(query, collection="docs", top_k=5):
    """Semantic search via MDDB vector endpoint."""
    resp = requests.post(f"{MDDB_URL}/v1/vector/search", json={
        "collection": collection,
        "query": query,
        "topK": top_k,
        "threshold": 0.6,
        "includeContent": True,
    })
    resp.raise_for_status()
    return resp.json().get("results", [])

def fts_search(query, collection="docs", limit=10):
    """Full-text search via MDDB."""
    resp = requests.post(f"{MDDB_URL}/v1/fts", json={
        "collection": collection,
        "query": query,
        "limit": limit,
    })
    resp.raise_for_status()
    return resp.json().get("results", [])

def ask(question, collection="docs"):
    """RAG: retrieve context from MDDB, then answer with Ollama."""
    results = search_mddb(question, collection)
    context = "\\n\\n---\\n\\n".join(
        f"## {r.get('key', 'unknown')}\\n{r.get('contentMd', '')[:2000]}"
        for r in results
    )
    prompt = f"""Answer the question using ONLY the context below.
If the context doesn't contain the answer, say so.

Context:
{context}

Question: {question}
Answer:"""

    response = ollama.chat(model=OLLAMA_MODEL, messages=[
        {"role": "user", "content": prompt},
    ])
    return response["message"]["content"]

if __name__ == "__main__":
    import sys
    question = " ".join(sys.argv[1:]) or "What is MDDB?"
    print(ask(question))
`,
        instructions: [
          'Install dependencies: pip install requests ollama',
          'Make sure Ollama is running: ollama serve',
          'Pull a model: ollama pull llama3.2',
          `Ensure MDDB is running at ${httpBase}`,
          'Run: python mddb_ollama_agent.py "your question here"',
        ],
      };

    case 'deepseek':
      return {
        filename: 'deepseek_mcp_config.json',
        content: JSON.stringify({
          mcpServers: {
            mddb: {
              command: 'docker',
              args: [
                'run', '-i', '--rm', '--network', 'host',
                '-v', 'mddb-data:/app/data',
                '-e', 'MDDB_MCP_STDIO=true',
                'tradik/mddb:latest',
              ],
              env: {},
            },
          },
        }, null, 2),
        instructions: [
          'DeepSeek supports MCP via compatible clients (Cline, Continue, etc.)',
          'Add this config to your MCP client settings (e.g., ~/.cline/mcp.json)',
          'Use DeepSeek as the LLM provider in your MCP client',
          'The MDDB tools will be available for document search and retrieval',
        ],
      };

    case 'manus':
      return {
        filename: 'manus_mddb_tool.yaml',
        content: `# Manus Agent - MDDB Tool Configuration
# Add this to your Manus agent's tool definitions

name: mddb_search
description: "Search the MDDB knowledge base for relevant documents"
type: api
endpoint: "${httpBase}/v1/vector/search"
method: POST
headers:
  Content-Type: "application/json"
body:
  collection: "docs"
  query: "{{input}}"
  topK: 5
  threshold: 0.6
  includeContent: true

---

name: mddb_fts
description: "Full-text search in MDDB documents"
type: api
endpoint: "${httpBase}/v1/fts"
method: POST
headers:
  Content-Type: "application/json"
body:
  collection: "docs"
  query: "{{input}}"
  limit: 10

---

name: mddb_get_document
description: "Retrieve a specific document from MDDB"
type: api
endpoint: "${httpBase}/v1/get"
method: POST
headers:
  Content-Type: "application/json"
body:
  collection: "{{collection}}"
  key: "{{key}}"`,
        instructions: [
          'Add these tool definitions to your Manus agent configuration',
          `Make sure MDDB is accessible at ${httpBase}`,
          'The agent can use semantic search, full-text search, and document retrieval',
          'Adjust collection names and parameters to match your data',
        ],
      };

    case 'bielik':
      return {
        filename: 'bielik_mddb_config.py',
        content: `#!/usr/bin/env python3
"""Bielik.ai + MDDB Integration

Connects Bielik Polish LLM to MDDB for RAG-powered responses.
Bielik excels at Polish language tasks and understanding.

Requirements:
  pip install requests
"""

import requests

MDDB_URL = "${httpBase}"
BIELIK_API_URL = "https://api.bielik.ai/v1"
BIELIK_API_KEY = "your-bielik-api-key"  # Get from https://bielik.ai
BIELIK_MODEL = "Bielik-11B-v2.3-Instruct"

def search_mddb(query, collection="docs", top_k=5):
    """Semantic search via MDDB."""
    resp = requests.post(f"{MDDB_URL}/v1/vector/search", json={
        "collection": collection,
        "query": query,
        "topK": top_k,
        "threshold": 0.6,
        "includeContent": True,
    })
    resp.raise_for_status()
    return resp.json().get("results", [])

def ask_bielik(question, collection="docs"):
    """RAG: retrieve from MDDB, answer with Bielik."""
    results = search_mddb(question, collection)
    context = "\\n\\n---\\n\\n".join(
        f"## {r.get('key', 'unknown')}\\n{r.get('contentMd', '')[:2000]}"
        for r in results
    )

    resp = requests.post(
        f"{BIELIK_API_URL}/chat/completions",
        headers={
            "Authorization": f"Bearer {BIELIK_API_KEY}",
            "Content-Type": "application/json",
        },
        json={
            "model": BIELIK_MODEL,
            "messages": [
                {"role": "system", "content": "Odpowiadaj na pytania korzystajac z podanego kontekstu. Jesli kontekst nie zawiera odpowiedzi, powiedz o tym."},
                {"role": "user", "content": f"Kontekst:\\n{context}\\n\\nPytanie: {question}"},
            ],
            "temperature": 0.3,
        },
    )
    resp.raise_for_status()
    return resp.json()["choices"][0]["message"]["content"]

if __name__ == "__main__":
    import sys
    question = " ".join(sys.argv[1:]) or "Czym jest MDDB?"
    print(ask_bielik(question))
`,
        instructions: [
          'Get your API key from https://bielik.ai',
          'Install dependencies: pip install requests',
          `Ensure MDDB is running at ${httpBase}`,
          'Set your BIELIK_API_KEY in the script',
          'Run: python bielik_mddb_config.py "Twoje pytanie tutaj"',
        ],
      };

    case 'openwebui':
      return {
        filename: 'mddb_model.py',
        content: `"""
title: MDDB RAG Model
author: MDDB
version: 2.6.0
license: MIT
description: RAG model using MDDB for document retrieval with multi-LLM support
"""

import json
import requests
from typing import Generator, Iterator, Union, List, Dict
from pydantic import BaseModel, Field


class Pipe:
    """MDDB RAG Model - retrieves documents from MDDB and answers using configurable LLM."""

    class Valves(BaseModel):
        mddbUrl: str = Field(
            default="${httpBase}",
            description="MDDB server URL (MCP endpoints at /mcp/*)"
        )
        collection: str = Field(
            default="docs",
            description="Default MDDB collection"
        )
        topK: int = Field(
            default=5,
            description="Number of documents to retrieve"
        )
        threshold: float = Field(
            default=0.5,
            description="Minimum similarity threshold for semantic search"
        )
        llmProvider: str = Field(
            default="ollama",
            description="LLM provider: ollama, openai, or deepseek"
        )
        llmModel: str = Field(
            default="llama3.2:latest",
            description="Model name (e.g. llama3.2:latest, gpt-4o, deepseek-chat)"
        )
        llmApiUrl: str = Field(
            default="http://ollama:11434",
            description="LLM API URL"
        )
        llmApiKey: str = Field(
            default="",
            description="API key for OpenAI/DeepSeek (not needed for Ollama)"
        )

    def __init__(self):
        self.type = "manifold"
        self.valves = self.Valves()
        self.systemPrompt = """You are a helpful assistant.
Answer questions using ONLY the context provided from the knowledge base.
If the context doesn't contain enough information to answer, say so.
Always be helpful, accurate, and provide specific details when available."""

    def pipes(self):
        return [{"id": "mddb-rag", "name": "MDDB RAG Assistant"}]

    def _vectorSearch(self, query: str) -> List[Dict]:
        """Vector search via MDDB REST API."""
        try:
            response = requests.post(
                f"{self.valves.mddbUrl}/v1/vector/search",
                json={
                    "collection": self.valves.collection,
                    "query": query,
                    "topK": self.valves.topK,
                    "threshold": self.valves.threshold,
                    "includeContent": True
                },
                timeout=10
            )
            if response.status_code == 200:
                return response.json().get("results", [])
        except Exception:
            pass
        return []

    def _ftsSearch(self, query: str) -> List[Dict]:
        """Full-text search via MDDB MCP endpoint."""
        try:
            response = requests.post(
                f"{self.valves.mddbUrl}/mcp/tools/call",
                json={
                    "name": "full_text_search",
                    "arguments": {
                        "collection": self.valves.collection,
                        "query": query,
                        "limit": self.valves.topK,
                        "algorithm": "bm25"
                    }
                },
                timeout=10
            )
            if response.status_code == 200:
                data = response.json()
                return data.get("content", []) if isinstance(data, dict) else []
        except Exception:
            pass
        return []

    def _formatContext(self, results: List[Dict]) -> str:
        if not results:
            return "No relevant documents found in the knowledge base."
        parts = []
        for i, doc in enumerate(results[:self.valves.topK], 1):
            key = doc.get("key", doc.get("id", "unknown"))
            content = doc.get("contentMd", doc.get("content", ""))
            score = doc.get("score", doc.get("similarity", 0))
            if len(content) > 2000:
                content = content[:2000] + "..."
            parts.append(f"## Document {i}: {key}\\n**Score:** {score:.2f}\\n\\n{content}")
        return "\\n\\n---\\n\\n".join(parts)

    def _callLLM(self, messages, stream=True):
        """Call LLM based on configured provider."""
        provider = self.valves.llmProvider.lower()

        if provider == "ollama":
            return self._callOllama(messages, stream)
        else:
            return self._callOpenAICompat(messages, stream)

    def _callOllama(self, messages, stream):
        payload = {"model": self.valves.llmModel, "messages": messages, "stream": stream}
        if stream:
            response = requests.post(f"{self.valves.llmApiUrl}/api/chat", json=payload, stream=True, timeout=120)
            response.raise_for_status()
            for line in response.iter_lines():
                if line:
                    try:
                        data = json.loads(line)
                        if "message" in data and "content" in data["message"]:
                            yield data["message"]["content"]
                        if data.get("done", False):
                            break
                    except json.JSONDecodeError:
                        continue
        else:
            payload["stream"] = False
            response = requests.post(f"{self.valves.llmApiUrl}/api/chat", json=payload, timeout=120)
            response.raise_for_status()
            yield response.json().get("message", {}).get("content", "")

    def _callOpenAICompat(self, messages, stream):
        headers = {"Content-Type": "application/json"}
        if self.valves.llmApiKey:
            headers["Authorization"] = f"Bearer {self.valves.llmApiKey}"
        payload = {"model": self.valves.llmModel, "messages": messages, "stream": stream}

        if stream:
            response = requests.post(f"{self.valves.llmApiUrl}/v1/chat/completions", headers=headers, json=payload, stream=True, timeout=120)
            response.raise_for_status()
            for line in response.iter_lines():
                if line:
                    line = line.decode("utf-8") if isinstance(line, bytes) else line
                    if line.startswith("data: "):
                        line = line[6:]
                    if line == "[DONE]":
                        break
                    try:
                        data = json.loads(line)
                        delta = data.get("choices", [{}])[0].get("delta", {})
                        if "content" in delta:
                            yield delta["content"]
                    except json.JSONDecodeError:
                        continue
        else:
            payload["stream"] = False
            response = requests.post(f"{self.valves.llmApiUrl}/v1/chat/completions", headers=headers, json=payload, timeout=120)
            response.raise_for_status()
            yield response.json()["choices"][0]["message"]["content"]

    def pipe(self, body: dict) -> Union[str, Generator, Iterator]:
        messages = body.get("messages", [])
        lastUserMsg = ""
        for msg in reversed(messages):
            if msg.get("role") == "user":
                lastUserMsg = msg.get("content", "")
                break

        results = []
        if lastUserMsg:
            results = self._vectorSearch(lastUserMsg)
            if not results:
                results = self._ftsSearch(lastUserMsg)

        context = self._formatContext(results)
        fullSystemPrompt = f"""{self.systemPrompt}

---
KNOWLEDGE BASE CONTEXT:

{context}

---
Use the context above to answer the user's question."""

        ragMessages = [{"role": "system", "content": fullSystemPrompt}]
        for msg in messages:
            if msg.get("role") != "system":
                ragMessages.append(msg)

        try:
            stream = body.get("stream", True)
            gen = self._callLLM(ragMessages, stream)
            if stream:
                return gen
            else:
                return "".join(gen)
        except Exception as e:
            return f"Error: {str(e)}"
`,
        instructions: [
          'Go to Open WebUI → Admin → Functions → Add new function',
          'Paste this script and save it',
          'Configure the Valves (settings) to match your LLM provider',
          'For Ollama: set llmApiUrl to your Ollama address',
          'For OpenAI: set llmProvider=openai, llmApiUrl=https://api.openai.com, llmApiKey=sk-...',
          'For DeepSeek: set llmProvider=deepseek, llmApiUrl=https://api.deepseek.com, llmApiKey=...',
        ],
      };

    default:
      return { filename: 'config.txt', content: '', instructions: [] };
  }
}

export default function MCPConfigPanel() {
  const { config } = useStore();
  const [activeTab, setActiveTab] = useState('mcp');
  const [copied, setCopied] = useState(false);
  const [showAlt, setShowAlt] = useState(false);
  const [mcpYaml, setMcpYaml] = useState(null);
  const [mcpLoading, setMcpLoading] = useState(true);

  useEffect(() => {
    loadMCPConfig();
  }, []);

  const loadMCPConfig = async () => {
    try {
      const text = await mddbClient.getMCPConfigText();
      setMcpYaml(text);
    } catch {
      // Will fallback to generated config
    } finally {
      setMcpLoading(false);
    }
  };

  const grpcAddr = config?.grpcAddr || ':11024';
  const httpAddr = config?.httpAddr || ':11023';
  const tabConfig = generateConfig(activeTab, grpcAddr, `http://localhost${httpAddr}`);

  // For MCP tab, prefer the live server config if available
  const displayContent = activeTab === 'mcp' && mcpYaml ? mcpYaml : (showAlt && tabConfig.alt ? tabConfig.alt.content : tabConfig.content);
  const displayFilename = showAlt && tabConfig.alt ? `${tabConfig.filename} (${tabConfig.alt.label})` : tabConfig.filename;

  const handleCopy = async () => {
    try {
      await navigator.clipboard.writeText(displayContent);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch (error) {
      console.error('Failed to copy:', error);
    }
  };

  const handleDownload = () => {
    const blob = new Blob([displayContent], { type: 'text/plain' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = tabConfig.filename;
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    URL.revokeObjectURL(url);
  };

  const currentTabInfo = TABS.find(t => t.id === activeTab);

  return (
    <div className="h-full overflow-y-auto bg-gray-50 p-6">
      <div className="max-w-4xl mx-auto">
        <div className="mb-6">
          <h1 className="text-2xl font-bold text-gray-900 mb-2">LLM Connections</h1>
          <p className="text-gray-600">Configuration for connecting MDDB to AI agents and LLM tools</p>
        </div>

        {/* Tabs */}
        <div className="bg-white rounded-lg shadow mb-6">
          <div className="border-b border-gray-200 overflow-x-auto">
            <div className="flex min-w-max">
              {TABS.map((tab) => (
                <button
                  key={tab.id}
                  onClick={() => { setActiveTab(tab.id); setShowAlt(false); setCopied(false); }}
                  className={`px-4 py-3 text-sm font-medium whitespace-nowrap border-b-2 transition-colors ${
                    activeTab === tab.id
                      ? 'border-primary-600 text-primary-700 bg-primary-50'
                      : 'border-transparent text-gray-500 hover:text-gray-700 hover:border-gray-300'
                  }`}
                >
                  {tab.label}
                </button>
              ))}
            </div>
          </div>

          {/* Tab Description */}
          <div className="px-4 py-3 border-b border-gray-100 flex items-center justify-between">
            <div className="flex items-center text-sm text-gray-600">
              <Brain className="w-4 h-4 mr-2 text-primary-600" />
              <span>{currentTabInfo?.desc}</span>
            </div>
            <div className="flex items-center space-x-2">
              {tabConfig.alt && (
                <button
                  onClick={() => setShowAlt(!showAlt)}
                  className="text-xs px-2 py-1 bg-gray-100 hover:bg-gray-200 rounded transition-colors"
                >
                  {showAlt ? 'Docker' : tabConfig.alt.label}
                </button>
              )}
              <button
                onClick={handleCopy}
                className="flex items-center px-3 py-1.5 text-sm bg-gray-100 hover:bg-gray-200 rounded transition-colors"
              >
                {copied ? (
                  <>
                    <Check className="w-3.5 h-3.5 mr-1 text-green-600" />
                    <span className="text-green-600">Copied!</span>
                  </>
                ) : (
                  <>
                    <Copy className="w-3.5 h-3.5 mr-1" />
                    <span>Copy</span>
                  </>
                )}
              </button>
              <button
                onClick={handleDownload}
                className="flex items-center px-3 py-1.5 text-sm bg-primary-600 text-white hover:bg-primary-700 rounded transition-colors"
              >
                <Download className="w-3.5 h-3.5 mr-1" />
                <span>Download</span>
              </button>
            </div>
          </div>

          {/* Code Block */}
          <div className="bg-gray-900">
            <div className="px-4 py-2 border-b border-gray-700 flex items-center">
              <Code className="w-3.5 h-3.5 text-gray-400 mr-2" />
              <span className="text-xs text-gray-400 font-mono">{displayFilename}</span>
            </div>
            <div className="p-4 overflow-x-auto">
              {activeTab === 'mcp' && mcpLoading ? (
                <div className="flex items-center justify-center py-8">
                  <div className="animate-spin rounded-full h-6 w-6 border-b-2 border-gray-400"></div>
                </div>
              ) : (
                <pre className="text-sm text-gray-100 font-mono whitespace-pre">
                  <code>{displayContent}</code>
                </pre>
              )}
            </div>
          </div>
        </div>

        {/* Instructions */}
        {tabConfig.instructions.length > 0 && (
          <div className="bg-blue-50 border border-blue-200 rounded-lg p-4 mb-6">
            <h3 className="text-sm font-semibold text-blue-900 mb-2">Setup Instructions:</h3>
            <ol className="text-sm text-blue-800 space-y-1 list-decimal list-inside">
              {tabConfig.instructions.map((step, i) => (
                <li key={i}>{step}</li>
              ))}
            </ol>
          </div>
        )}

        {/* Documentation Link */}
        <div className="bg-gray-50 border border-gray-200 rounded-lg p-4">
          <p className="text-sm text-gray-700">
            For detailed setup guides, see the{' '}
            <a
              href="https://github.com/tradik/mddb/blob/main/docs/LLM_CONNECTIONS.md"
              target="_blank"
              rel="noopener noreferrer"
              className="text-primary-600 hover:text-primary-700 underline"
            >
              LLM Connections documentation
            </a>.
          </p>
        </div>
      </div>
    </div>
  );
}
