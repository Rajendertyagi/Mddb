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

  // Search mode
  searchMode: 'metadata',

  // Vector search
  vectorQuery: '',
  vectorTopK: 10,
  vectorThreshold: 0.0,
  vectorResults: [],
  vectorLoading: false,
  vectorError: null,
  vectorStats: null,

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
  setVectorQuery: (q) => set({ vectorQuery: q }),
  setVectorTopK: (k) => set({ vectorTopK: k }),
  setVectorThreshold: (t) => set({ vectorThreshold: t }),
  setVectorResults: (r) => set({ vectorResults: r }),
  setVectorLoading: (l) => set({ vectorLoading: l }),
  setVectorError: (e) => set({ vectorError: e }),
  setVectorStats: (s) => set({ vectorStats: s }),

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
