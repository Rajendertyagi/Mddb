/**
 * Global state management with Zustand
 */
import { create } from 'zustand';

export const useStore = create((set, get) => ({
  // Server stats
  stats: null,
  statsLoading: false,
  statsError: null,

  // Current collection
  currentCollection: null,
  
  // Documents
  documents: [],
  documentsLoading: false,
  documentsError: null,
  
  // Current document
  currentDocument: null,
  currentDocumentLoading: false,
  currentDocumentError: null,
  
  // Filters
  filters: {},
  sortBy: 'addedAt',
  sortAsc: false,
  limit: 100,

  // Search mode ('metadata', 'fulltext', 'vector')
  searchMode: 'metadata',

  // API mode (rest or graphql)
  apiMode: localStorage.getItem('apiMode') || 'rest',

  // Full-text search
  ftsQuery: '',
  ftsLimit: 50,
  ftsAlgorithm: 'tfidf',
  ftsFuzzy: 0,
  ftsResults: [],
  ftsLoading: false,
  ftsError: null,

  // Vector search
  vectorQuery: '',
  vectorTopK: 10,
  vectorThreshold: 0.0,
  vectorAlgorithm: 'flat',
  vectorResults: [],
  vectorLoading: false,
  vectorError: null,
  vectorStats: null,

  // Sidebar
  sidebarWidth: parseInt(localStorage.getItem('sidebarWidth')) || 256,
  sidebarCollapsed: localStorage.getItem('sidebarCollapsed') === 'true',

  // View mode (documents, system, config, mcp, endpoints, users, groups)
  viewMode: 'documents',

  // System info
  systemInfo: null,
  systemInfoLoading: false,
  systemInfoError: null,

  // Config
  config: null,
  configLoading: false,
  configError: null,

  // MCP Config
  mcpConfig: null,
  mcpConfigLoading: false,
  mcpConfigError: null,

  // Endpoints
  endpoints: null,
  endpointsLoading: false,
  endpointsError: null,

  // Users
  users: [],
  usersLoading: false,
  usersError: null,

  // Groups
  groups: [],
  groupsLoading: false,
  groupsError: null,

  // Actions
  setStats: (stats) => set({ stats }),
  setStatsLoading: (loading) => set({ statsLoading: loading }),
  setStatsError: (error) => set({ statsError: error }),

  setCurrentCollection: (collection) => set({ currentCollection: collection }),
  
  setDocuments: (documents) => set({ documents }),
  setDocumentsLoading: (loading) => set({ documentsLoading: loading }),
  setDocumentsError: (error) => set({ documentsError: error }),
  
  setCurrentDocument: (doc) => set({ currentDocument: doc }),
  setCurrentDocumentLoading: (loading) => set({ currentDocumentLoading: loading }),
  setCurrentDocumentError: (error) => set({ currentDocumentError: error }),
  
  setFilters: (filters) => set({ filters }),
  setSortBy: (sortBy) => set({ sortBy }),
  setSortAsc: (asc) => set({ sortAsc: asc }),
  setLimit: (limit) => set({ limit }),

  setSearchMode: (mode) => set({ searchMode: mode }),
  setApiMode: (mode) => {
    localStorage.setItem('apiMode', mode);
    set({ apiMode: mode });
  },
  setFtsQuery: (q) => set({ ftsQuery: q }),
  setFtsLimit: (l) => set({ ftsLimit: l }),
  setFtsAlgorithm: (a) => set({ ftsAlgorithm: a }),
  setFtsFuzzy: (f) => set({ ftsFuzzy: f }),
  setFtsResults: (r) => set({ ftsResults: r }),
  setFtsLoading: (l) => set({ ftsLoading: l }),
  setFtsError: (e) => set({ ftsError: e }),

  setVectorQuery: (q) => set({ vectorQuery: q }),
  setVectorTopK: (k) => set({ vectorTopK: k }),
  setVectorThreshold: (t) => set({ vectorThreshold: t }),
  setVectorAlgorithm: (a) => set({ vectorAlgorithm: a }),
  setVectorResults: (r) => set({ vectorResults: r }),
  setVectorLoading: (l) => set({ vectorLoading: l }),
  setVectorError: (e) => set({ vectorError: e }),
  setVectorStats: (s) => set({ vectorStats: s }),

  // Sidebar actions
  setSidebarWidth: (w) => { localStorage.setItem('sidebarWidth', w); set({ sidebarWidth: w }); },
  setSidebarCollapsed: (c) => { localStorage.setItem('sidebarCollapsed', c); set({ sidebarCollapsed: c }); },

  // View mode actions
  setViewMode: (mode) => set({ viewMode: mode }),

  // System info actions
  setSystemInfo: (info) => set({ systemInfo: info }),
  setSystemInfoLoading: (loading) => set({ systemInfoLoading: loading }),
  setSystemInfoError: (error) => set({ systemInfoError: error }),

  // Config actions
  setConfig: (config) => set({ config }),
  setConfigLoading: (loading) => set({ configLoading: loading }),
  setConfigError: (error) => set({ configError: error }),

  // MCP Config actions
  setMcpConfig: (config) => set({ mcpConfig: config }),
  setMcpConfigLoading: (loading) => set({ mcpConfigLoading: loading }),
  setMcpConfigError: (error) => set({ mcpConfigError: error }),

  // Endpoints actions
  setEndpoints: (endpoints) => set({ endpoints }),
  setEndpointsLoading: (loading) => set({ endpointsLoading: loading }),
  setEndpointsError: (error) => set({ endpointsError: error }),

  // Users actions
  setUsers: (users) => set({ users }),
  setUsersLoading: (loading) => set({ usersLoading: loading }),
  setUsersError: (error) => set({ usersError: error }),

  // Groups actions
  setGroups: (groups) => set({ groups }),
  setGroupsLoading: (loading) => set({ groupsLoading: loading }),
  setGroupsError: (error) => set({ groupsError: error }),

  // Clear current document
  clearCurrentDocument: () => set({ 
    currentDocument: null, 
    currentDocumentError: null 
  }),

  // Delete document
  deleteDocument: async (collection, key, lang) => {
    try {
      const mddbClient = await import('../lib/mddb-client').then(m => m.default);
      await mddbClient.deleteDocument({ collection, key, lang });
      
      // Remove from documents list
      const { documents, currentDocument } = get();
      const updatedDocuments = documents.filter(doc => 
        !(doc.key === key && doc.lang === lang)
      );
      set({ documents: updatedDocuments });
      
      // Clear current document if it was the deleted one
      if (currentDocument?.key === key && currentDocument?.lang === lang) {
        set({ currentDocument: null });
      }
      
      return true;
    } catch (error) {
      console.error('Failed to delete document:', error);
      throw error;
    }
  },
}));
