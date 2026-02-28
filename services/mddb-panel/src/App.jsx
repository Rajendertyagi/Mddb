import { useEffect } from 'react';
import { useStore } from './lib/store';
import mddbClient from './lib/mddb-client';
import Header from './components/Header';
import Sidebar from './components/Sidebar';
import DocumentList from './components/DocumentList';
import DocumentViewer from './components/DocumentViewer';
import VectorSearchPanel from './components/VectorSearchPanel';

function App() {
  const {
    stats,
    statsError,
    setStats,
    setStatsLoading,
    setStatsError,
    currentDocument,
    searchMode,
  } = useStore();

  useEffect(() => {
    loadStats();
  }, []);

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

  return (
    <div className="min-h-screen bg-gray-50">
      <Header onRefresh={loadStats} />

      <div className="flex" style={{ height: 'calc(100vh - 64px)' }}>
        <Sidebar stats={stats} statsError={statsError} />

        <div className="flex-1 flex">
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
        </div>
      </div>
    </div>
  );
}

export default App;
