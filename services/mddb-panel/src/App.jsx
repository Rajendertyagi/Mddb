import { useEffect, useState } from 'react';
import { useStore } from './lib/store';
import mddbClient from './lib/mddb-client';
import { authManager } from './lib/auth';
import Header from './components/Header';
import Sidebar from './components/Sidebar';
import DocumentList from './components/DocumentList';
import DocumentViewer from './components/DocumentViewer';
import VectorSearchPanel from './components/VectorSearchPanel';
import LoginForm from './components/LoginForm';
import SystemInfo from './components/SystemInfo';
import ConfigPanel from './components/ConfigPanel';
import MCPConfigPanel from './components/MCPConfigPanel';
import EndpointsPanel from './components/EndpointsPanel';
import UsersPanel from './components/UsersPanel';
import GroupsPanel from './components/GroupsPanel';
import VectorPanel from './components/VectorPanel';

function App() {
  const {
    stats,
    statsError,
    setStats,
    setStatsLoading,
    setStatsError,
    currentDocument,
    searchMode,
    viewMode,
  } = useStore();

  const [isAuthenticated, setIsAuthenticated] = useState(authManager.isAuthenticated());
  const [needsAuth, setNeedsAuth] = useState(false);

  useEffect(() => {
    checkAuthAndLoadStats();
  }, []);

  const checkAuthAndLoadStats = async () => {
    setStatsLoading(true);
    setStatsError(null);
    try {
      const data = await mddbClient.getStats();
      setStats(data);
      setIsAuthenticated(true);
      setNeedsAuth(false);
    } catch (error) {
      // If we get Unauthorized error, auth is enabled
      if (error.message.includes('401') || error.message.includes('Unauthorized')) {
        setNeedsAuth(true);
        setIsAuthenticated(false);
      } else {
        // Other errors - auth might be disabled
        setStatsError(error.message);
        setIsAuthenticated(true); // Assume auth is disabled
        setNeedsAuth(false);
      }
      console.error('Failed to load stats:', error);
    } finally {
      setStatsLoading(false);
    }
  };

  const loadStats = async () => {
    setStatsLoading(true);
    setStatsError(null);
    try {
      const data = await mddbClient.getStats();
      setStats(data);
    } catch (error) {
      setStatsError(error.message);
      console.error('Failed to load stats:', error);
    } finally {
      setStatsLoading(false);
    }
  };

  // Show login form if authentication is required and user is not authenticated
  if (needsAuth && !isAuthenticated) {
    return <LoginForm onSuccess={() => {
      setIsAuthenticated(true);
      setNeedsAuth(false);
      loadStats();
    }} />;
  }

  return (
    <div className="min-h-screen bg-gray-50">
      <Header onRefresh={loadStats} />

      <div className="flex" style={{ height: 'calc(100vh - 64px)' }}>
        <Sidebar stats={stats} statsError={statsError} />

        <div className="flex-1 flex">
          {viewMode === 'system' && (
            <div className="flex-1 border-l border-gray-200">
              <SystemInfo />
            </div>
          )}
          {viewMode === 'config' && (
            <div className="flex-1 border-l border-gray-200">
              <ConfigPanel />
            </div>
          )}
          {viewMode === 'mcp' && (
            <div className="flex-1 border-l border-gray-200">
              <MCPConfigPanel />
            </div>
          )}
          {viewMode === 'endpoints' && (
            <div className="flex-1 border-l border-gray-200">
              <EndpointsPanel />
            </div>
          )}
          {viewMode === 'users' && (
            <div className="flex-1 border-l border-gray-200">
              <UsersPanel />
            </div>
          )}
          {viewMode === 'groups' && (
            <div className="flex-1 border-l border-gray-200">
              <GroupsPanel />
            </div>
          )}
          {viewMode === 'vectors' && (
            <div className="flex-1 border-l border-gray-200">
              <VectorPanel />
            </div>
          )}
          {viewMode === 'documents' && (
            <>
              {searchMode === 'vector' ? (
                <>
                  <div className="flex-1 border-l border-gray-200">
                    <VectorSearchPanel />
                  </div>
                  {currentDocument && (
                    <div className="flex-1 border-l border-gray-200">
                      <DocumentViewer />
                    </div>
                  )}
                </>
              ) : (
                <>
                  <div className="flex-1 border-l border-gray-200">
                    <DocumentList />
                  </div>
                  {currentDocument && (
                    <div className="flex-1 border-l border-gray-200">
                      <DocumentViewer />
                    </div>
                  )}
                </>
              )}
            </>
          )}
        </div>
      </div>
    </div>
  );
}

export default App;
