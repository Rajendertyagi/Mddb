import { useEffect, useState } from 'react';
import { Code, Copy, Download, Check } from 'lucide-react';
import { useStore } from '../lib/store';
import mddbClient from '../lib/mddb-client';

export default function MCPConfigPanel() {
  const {
    mcpConfig,
    mcpConfigLoading,
    mcpConfigError,
    setMcpConfig,
    setMcpConfigLoading,
    setMcpConfigError,
  } = useStore();

  const [copied, setCopied] = useState(false);

  useEffect(() => {
    loadMCPConfig();
  }, []);

  const loadMCPConfig = async () => {
    setMcpConfigLoading(true);
    setMcpConfigError(null);
    try {
      const response = await fetch('/v1/mcp/config', {
        headers: {
          'Authorization': `Bearer ${localStorage.getItem('mddb_auth_token')}`,
        },
      });
      if (!response.ok) throw new Error('Failed to load MCP config');
      const text = await response.text();
      setMcpConfig(text);
    } catch (error) {
      setMcpConfigError(error.message);
      console.error('Failed to load MCP config:', error);
    } finally {
      setMcpConfigLoading(false);
    }
  };

  const handleCopy = async () => {
    try {
      await navigator.clipboard.writeText(mcpConfig);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch (error) {
      console.error('Failed to copy:', error);
    }
  };

  const handleDownload = () => {
    const blob = new Blob([mcpConfig], { type: 'text/yaml' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = 'mddb-mcp-config.yaml';
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    URL.revokeObjectURL(url);
  };

  if (mcpConfigLoading) {
    return (
      <div className="flex items-center justify-center h-full">
        <div className="animate-spin rounded-full h-12 w-12 border-b-2 border-primary-600"></div>
      </div>
    );
  }

  if (mcpConfigError) {
    return (
      <div className="flex items-center justify-center h-full">
        <div className="text-center">
          <div className="text-red-600 font-medium mb-2">Failed to load MCP configuration</div>
          <div className="text-sm text-gray-500">{mcpConfigError}</div>
          <button
            onClick={loadMCPConfig}
            className="mt-4 px-4 py-2 bg-primary-600 text-white rounded hover:bg-primary-700"
          >
            Retry
          </button>
        </div>
      </div>
    );
  }

  if (!mcpConfig) return null;

  return (
    <div className="h-full overflow-y-auto bg-gray-50 p-6">
      <div className="max-w-4xl mx-auto">
        <div className="mb-6">
          <h1 className="text-2xl font-bold text-gray-900 mb-2">MCP Configuration</h1>
          <p className="text-gray-600">Model Context Protocol server configuration in YAML format</p>
        </div>

        {/* Actions */}
        <div className="bg-white rounded-lg shadow mb-6 p-4">
          <div className="flex items-center justify-between">
            <div className="flex items-center text-sm text-gray-600">
              <Code className="w-4 h-4 mr-2" />
              <span>config.yaml</span>
            </div>
            <div className="flex space-x-2">
              <button
                onClick={handleCopy}
                className="flex items-center px-3 py-2 text-sm bg-gray-100 hover:bg-gray-200 rounded transition-colors"
              >
                {copied ? (
                  <>
                    <Check className="w-4 h-4 mr-1 text-green-600" />
                    <span className="text-green-600">Copied!</span>
                  </>
                ) : (
                  <>
                    <Copy className="w-4 h-4 mr-1" />
                    <span>Copy</span>
                  </>
                )}
              </button>
              <button
                onClick={handleDownload}
                className="flex items-center px-3 py-2 text-sm bg-primary-600 text-white hover:bg-primary-700 rounded transition-colors"
              >
                <Download className="w-4 h-4 mr-1" />
                <span>Download</span>
              </button>
            </div>
          </div>
        </div>

        {/* YAML Content */}
        <div className="bg-white rounded-lg shadow overflow-hidden">
          <div className="bg-gray-800 px-4 py-3 border-b border-gray-700">
            <span className="text-sm font-medium text-gray-300">YAML Configuration</span>
          </div>
          <div className="p-4 bg-gray-900">
            <pre className="text-sm text-gray-100 font-mono overflow-x-auto">
              <code>{mcpConfig}</code>
            </pre>
          </div>
        </div>

        {/* Instructions */}
        <div className="mt-6 bg-blue-50 border border-blue-200 rounded-lg p-4">
          <h3 className="text-sm font-semibold text-blue-900 mb-2">Using this configuration:</h3>
          <ol className="text-sm text-blue-800 space-y-1 list-decimal list-inside">
            <li>Save this configuration to <code className="bg-blue-100 px-1 rounded">config.yaml</code></li>
            <li>Place it in the mddb-mcp service directory</li>
            <li>Start the MCP server: <code className="bg-blue-100 px-1 rounded">mddb-mcp</code></li>
            <li>The server will be available at the configured listen address</li>
          </ol>
        </div>

        {/* Documentation Link */}
        <div className="mt-4 bg-gray-50 border border-gray-200 rounded-lg p-4">
          <p className="text-sm text-gray-700">
            For more information about MCP tools and custom configuration, see the{' '}
            <a
              href="https://github.com/tradik/mddb#mcp-server"
              target="_blank"
              rel="noopener noreferrer"
              className="text-primary-600 hover:text-primary-700 underline"
            >
              MCP documentation
            </a>.
          </p>
        </div>
      </div>
    </div>
  );
}
