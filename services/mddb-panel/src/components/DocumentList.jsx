import { useEffect, useState } from 'react';
import { FileText, Calendar, Tag, Trash2, X, Upload } from 'lucide-react';
import { format } from 'date-fns';
import { useStore } from '../lib/store';
import mddbClient from '../lib/mddb-client';

export default function DocumentList() {
  const [deleteConfirm, setDeleteConfirm] = useState(null);
  const [showUploadModal, setShowUploadModal] = useState(false);
  const {
    currentCollection,
    documents,
    documentsLoading,
    documentsError,
    setDocuments,
    setDocumentsLoading,
    setDocumentsError,
    currentDocument,
    setCurrentDocument,
    filters,
    sortBy,
    sortAsc,
    limit,
    deleteDocument,
    setStats,
    setCurrentCollection,
  } = useStore();

  useEffect(() => {
    if (currentCollection) {
      loadDocuments();
    }
  }, [currentCollection, filters, sortBy, sortAsc, limit]);

  const loadDocuments = async () => {
    setDocumentsLoading(true);
    setDocumentsError(null);
    try {
      const data = await mddbClient.search({
        collection: currentCollection,
        filterMeta: filters,
        sort: sortBy,
        asc: sortAsc,
        limit,
      });
      // API returns array directly, not { documents: [...] }
      // Add collection field to each document for editing
      const documentsWithCollection = Array.isArray(data) 
        ? data.map(doc => ({ ...doc, collection: currentCollection }))
        : [];
      setDocuments(documentsWithCollection);
    } catch (error) {
      // Handle corrupted data errors
      if (error.message.includes('invalid character')) {
        setDocumentsError('Collection contains corrupted data. This collection may need to be recreated.');
      } else {
        setDocumentsError(error.message);
      }
      console.error('Failed to load documents:', error);
      setDocuments([]);
    } finally {
      setDocumentsLoading(false);
    }
  };

  const handleDocumentClick = async (doc) => {
    // Set the document immediately with basic info
    const initialDocument = {
      ...doc,
      collection: currentCollection,
      contentMd: doc.contentMd || 'Loading content...'
    };
    
    setCurrentDocument(initialDocument);
    
    // Then try to fetch full content in background
    try {
      const fullDocument = await mddbClient.getDocument({
        collection: currentCollection,
        key: doc.key,
        lang: doc.lang
      });
      
      // Update with full content if different
      if (fullDocument.contentMd && fullDocument.contentMd !== doc.contentMd) {
        const documentWithCollection = {
          ...fullDocument,
          collection: currentCollection
        };
        setCurrentDocument(documentWithCollection);
      }
    } catch (error) {
      console.error('Failed to load full document content:', error);
      // Update with error message
      const errorDocument = {
        ...initialDocument,
        contentMd: `Error loading document content: ${error.message}. Please try again.`
      };
      setCurrentDocument(errorDocument);
    }
  };

  const handleDelete = async (doc, e) => {
    e.stopPropagation(); // Prevent document click
    setDeleteConfirm(doc);
  };

  const confirmDelete = async (doc) => {
    try {
      await deleteDocument(currentCollection, doc.key, doc.lang);
      setDeleteConfirm(null);
    } catch (error) {
      console.error('Failed to delete document:', error);
      // You could add error handling here (toast, alert, etc.)
    }
  };

  if (documentsLoading) {
    return (
      <div className="flex items-center justify-center h-full">
        <div className="text-center">
          <div className="animate-spin rounded-full h-12 w-12 border-b-2 border-blue-600 mx-auto mb-4"></div>
          <p className="text-gray-500">Loading documents...</p>
        </div>
      </div>
    );
  }

  if (documentsError) {
    return (
      <div className="flex items-center justify-center h-full">
        <div className="text-center">
          <p className="text-red-600 mb-2">Error loading documents</p>
          <p className="text-sm text-gray-500">{documentsError}</p>
        </div>
      </div>
    );
  }

  return (
    <div className="h-full flex flex-col">
      <div className="p-4 border-b border-gray-200">
        <div className="flex items-center justify-between">
          <h3 className="text-sm font-semibold text-gray-700">
            {documents.length} Documents
          </h3>
          <div className="flex items-center gap-2">
            <button
              onClick={() => setShowUploadModal(true)}
              className="px-3 py-1.5 bg-blue-600 text-white text-xs rounded hover:bg-blue-700 flex items-center gap-1.5"
            >
              <Upload className="w-3.5 h-3.5" />
              Upload
            </button>
            <button
              onClick={loadDocuments}
              className="text-xs text-blue-600 hover:text-blue-700"
            >
              Refresh
            </button>
          </div>
        </div>
      </div>

      <div className="flex-1 overflow-y-auto">
        {documents.length === 0 ? (
          <div className="flex items-center justify-center h-full">
            <div className="text-center">
              <FileText className="w-12 h-12 text-gray-400 mx-auto mb-2" />
              <p className="text-gray-500">No documents found</p>
            </div>
          </div>
        ) : (
          <div className="divide-y divide-gray-200">
            {documents.map((doc) => (
              <button
                key={`${doc.key}-${doc.lang}`}
                onClick={() => handleDocumentClick(doc)}
                className={`w-full text-left p-4 hover:bg-gray-50 transition-colors ${
                  currentDocument?.key === doc.key && currentDocument?.lang === doc.lang
                    ? 'bg-blue-50 border-l-4 border-blue-600'
                    : ''
                }`}
              >
                <div className="flex items-start justify-between mb-2">
                  <h4 className="font-medium text-gray-900 truncate flex-1">
                    {doc.key}
                  </h4>
                  <div className="flex items-center gap-2">
                    <span className="text-xs text-gray-500">{doc.lang}</span>
                    <button
                      onClick={(e) => handleDelete(doc, e)}
                      className="p-1 text-gray-400 hover:text-red-600 hover:bg-red-50 rounded transition-colors"
                      title="Delete document"
                    >
                      <Trash2 className="w-4 h-4" />
                    </button>
                  </div>
                </div>
                
                <div className="flex items-center space-x-3 text-xs text-gray-500">
                  <div className="flex items-center space-x-1">
                    <Calendar className="w-3 h-3" />
                    <span>
                      {doc.updatedAt ? format(new Date(doc.updatedAt), 'MMM d, yyyy') : 'N/A'}
                    </span>
                  </div>
                  {doc.meta && Object.keys(doc.meta).length > 0 && (
                    <div className="flex items-center space-x-1">
                      <Tag className="w-3 h-3" />
                      <span>{Object.keys(doc.meta).length} tags</span>
                    </div>
                  )}
                </div>

                {doc.meta && Object.entries(doc.meta).slice(0, 2).map(([key, values]) => (
                  <div key={key} className="mt-2">
                    <span className="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium bg-gray-100 text-gray-800">
                      {key}: {Array.isArray(values) ? values.join(', ') : values}
                    </span>
                  </div>
                ))}
              </button>
            ))}
          </div>
        )}
      </div>

      {/* Delete Confirmation Modal */}
      {deleteConfirm && (
        <div className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50">
          <div className="bg-white rounded-lg p-6 max-w-sm mx-4">
            <div className="flex items-center mb-4">
              <Trash2 className="w-6 h-6 text-red-600 mr-3" />
              <h3 className="text-lg font-semibold text-gray-900">Delete Document</h3>
            </div>
            <p className="text-gray-600 mb-6">
              Are you sure you want to delete "{deleteConfirm.key}" ({deleteConfirm.lang})? This action cannot be undone.
            </p>
            <div className="flex justify-end gap-3">
              <button
                onClick={() => setDeleteConfirm(null)}
                className="px-4 py-2 text-gray-700 bg-gray-100 hover:bg-gray-200 rounded-lg transition-colors"
              >
                Cancel
              </button>
              <button
                onClick={() => confirmDelete(deleteConfirm)}
                className="px-4 py-2 text-white bg-red-600 hover:bg-red-700 rounded-lg transition-colors"
              >
                Delete
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Upload Modal */}
      {showUploadModal && (
        <UploadModal
          collection={currentCollection}
          onClose={() => setShowUploadModal(false)}
          onSuccess={async (uploadedCollection) => {
            setShowUploadModal(false);

            // Refresh stats to show new collection in sidebar
            try {
              const stats = await mddbClient.getStats();
              setStats(stats);

              // If no collection was selected, select the uploaded collection
              if (!currentCollection && uploadedCollection) {
                setCurrentCollection(uploadedCollection);
              } else {
                // Refresh current collection's documents
                loadDocuments();
              }
            } catch (error) {
              console.error('Failed to refresh stats:', error);
              // Still try to load documents even if stats refresh fails
              loadDocuments();
            }
          }}
        />
      )}
    </div>
  );
}

// Upload Modal Component
function UploadModal({ collection, onClose, onSuccess }) {
  const [file, setFile] = useState(null);
  const [key, setKey] = useState('');
  const [lang, setLang] = useState('en');
  const [meta, setMeta] = useState({});
  const [contentMd, setContentMd] = useState('');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(null);
  const [dragActive, setDragActive] = useState(false);

  const parseFrontmatter = (text) => {
    const frontmatterRegex = /^---\n([\s\S]*?)\n---\n([\s\S]*)$/;
    const match = text.match(frontmatterRegex);

    if (match) {
      const frontmatter = match[1];
      const content = match[2];

      // Parse YAML frontmatter (simple key:value parser)
      const metadata = {};
      frontmatter.split('\n').forEach(line => {
        const colonIndex = line.indexOf(':');
        if (colonIndex > 0) {
          const key = line.substring(0, colonIndex).trim();
          const value = line.substring(colonIndex + 1).trim();

          // Remove quotes if present
          const cleanValue = value.replace(/^["'](.*)["']$/, '$1');

          // IMPORTANT: Backend expects all meta values to be arrays of strings
          // Handle arrays (simple comma-separated)
          if (cleanValue.includes(',')) {
            metadata[key] = cleanValue.split(',').map(v => v.trim());
          } else {
            // Single value - wrap in array
            metadata[key] = [cleanValue];
          }
        }
      });

      return { metadata, content };
    }

    return { metadata: {}, content: text };
  };

  const handleFile = (selectedFile) => {
    if (!selectedFile) return;

    if (!selectedFile.name.endsWith('.md')) {
      setError('Only .md (markdown) files are supported');
      return;
    }

    // Check file size (max 10MB)
    const maxSize = 10 * 1024 * 1024;
    if (selectedFile.size > maxSize) {
      setError('File too large. Maximum size is 10MB');
      return;
    }

    setFile(selectedFile);
    setError(null);

    // Read file content
    const reader = new FileReader();
    reader.onload = (e) => {
      const text = e.target.result;

      // Check content size
      if (text.length > 5 * 1024 * 1024) {
        setError('Content too large. Maximum 5MB of text');
        setFile(null);
        return;
      }

      const { metadata, content } = parseFrontmatter(text);

      // Set content
      setContentMd(content);

      // Set metadata
      setMeta(metadata);

      // Set key from filename (without .md extension)
      const filename = selectedFile.name.replace(/\.md$/, '');
      setKey(filename);
    };
    reader.onerror = () => {
      setError('Failed to read file');
    };
    reader.readAsText(selectedFile);
  };

  const handleDrag = (e) => {
    e.preventDefault();
    e.stopPropagation();
    if (e.type === 'dragenter' || e.type === 'dragover') {
      setDragActive(true);
    } else if (e.type === 'dragleave') {
      setDragActive(false);
    }
  };

  const handleDrop = (e) => {
    e.preventDefault();
    e.stopPropagation();
    setDragActive(false);

    if (e.dataTransfer.files && e.dataTransfer.files.length > 0) {
      handleFile(e.dataTransfer.files[0]);
    }
  };

  const handleSubmit = async (e) => {
    e.preventDefault();
    setError(null);
    setLoading(true);

    // Ensure all required fields are present (handle null/undefined with defaults)
    const finalLang = (lang || '').trim() || 'en';
    const finalKey = (key || '').trim() || `doc-${Date.now()}`;
    const finalCollection = (collection || '').trim() || 'default';

    // Validate we have at least something to upload
    if (!contentMd || contentMd.length === 0) {
      setError('No content to upload. Please select a valid markdown file.');
      setLoading(false);
      return;
    }

    console.log('Starting upload...', {
      collection: finalCollection,
      key: finalKey,
      lang: finalLang,
      metaKeys: Object.keys(meta),
      contentLength: contentMd.length,
    });

    const startTime = Date.now();

    // Create timeout promise
    const timeout = new Promise((_, reject) =>
      setTimeout(() => reject(new Error('Upload timeout - server took too long to respond (>30s)')), 30000)
    );

    try {
      // Race between upload and timeout
      await Promise.race([
        mddbClient.addDocument({
          collection: finalCollection,
          key: finalKey,
          lang: finalLang,
          meta,
          contentMd,
        }),
        timeout
      ]);

      const duration = Date.now() - startTime;
      console.log(`Upload successful in ${duration}ms to collection: ${finalCollection}`);
      onSuccess(finalCollection);
    } catch (err) {
      const duration = Date.now() - startTime;
      console.error(`Upload error after ${duration}ms:`, err);
      setError(err.message || 'Upload failed');
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50 p-4">
      <div className="bg-white rounded-lg shadow-xl w-full max-w-2xl max-h-[90vh] overflow-y-auto">
        <div className="p-6">
          <div className="flex items-center justify-between mb-4">
            <h2 className="text-xl font-bold text-gray-900">Upload Markdown Document</h2>
            <button onClick={onClose} className="text-gray-400 hover:text-gray-600">
              <X className="w-5 h-5" />
            </button>
          </div>

          <form onSubmit={handleSubmit}>
            {/* File Drop Zone */}
            <div
              onDragEnter={handleDrag}
              onDragLeave={handleDrag}
              onDragOver={handleDrag}
              onDrop={handleDrop}
              className={`mb-4 border-2 border-dashed rounded-lg p-8 text-center transition-colors ${
                dragActive
                  ? 'border-blue-500 bg-blue-50'
                  : 'border-gray-300 hover:border-gray-400'
              }`}
            >
              {file ? (
                <div>
                  <FileText className="w-12 h-12 text-blue-600 mx-auto mb-2" />
                  <p className="text-sm font-medium text-gray-900">{file.name}</p>
                  <p className="text-xs text-gray-500 mt-1">
                    {(file.size / 1024).toFixed(1)} KB
                  </p>
                  <button
                    type="button"
                    onClick={() => {
                      setFile(null);
                      setKey('');
                      setLang('en');
                      setMeta({});
                      setContentMd('');
                    }}
                    className="mt-2 text-xs text-blue-600 hover:text-blue-700"
                  >
                    Choose different file
                  </button>
                </div>
              ) : (
                <div>
                  <Upload className="w-12 h-12 text-gray-400 mx-auto mb-2" />
                  <p className="text-sm text-gray-700 mb-1">
                    Drag and drop a .md file here
                  </p>
                  <p className="text-xs text-gray-500 mb-3">or</p>
                  <label className="inline-block px-4 py-2 bg-blue-600 text-white text-sm rounded hover:bg-blue-700 cursor-pointer">
                    Browse Files
                    <input
                      type="file"
                      accept=".md"
                      onChange={(e) => handleFile(e.target.files[0])}
                      className="hidden"
                    />
                  </label>
                </div>
              )}
            </div>

            {file && (
              <div className="space-y-4">
                <div>
                  <label className="block text-sm font-medium text-gray-700 mb-1">
                    Collection
                  </label>
                  <input
                    type="text"
                    value={collection}
                    disabled
                    className="w-full px-3 py-2 border border-gray-300 rounded-md bg-gray-50 text-gray-500"
                  />
                </div>

                <div className="grid grid-cols-2 gap-4">
                  <div>
                    <label className="block text-sm font-medium text-gray-700 mb-1">
                      Document Key *
                    </label>
                    <input
                      type="text"
                      value={key}
                      onChange={(e) => setKey(e.target.value)}
                      className="w-full px-3 py-2 border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500"
                      required
                    />
                  </div>

                  <div>
                    <label className="block text-sm font-medium text-gray-700 mb-1">
                      Language *
                    </label>
                    <input
                      type="text"
                      value={lang}
                      onChange={(e) => setLang(e.target.value)}
                      className="w-full px-3 py-2 border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500"
                      placeholder="en"
                      required
                    />
                    <p className="mt-1 text-xs text-gray-500">Default: en</p>
                  </div>
                </div>

                {Object.keys(meta).length > 0 && (
                  <div>
                    <label className="block text-sm font-medium text-gray-700 mb-1">
                      Metadata (from frontmatter)
                    </label>
                    <div className="p-3 bg-gray-50 border border-gray-200 rounded-md">
                      <pre className="text-xs text-gray-700 whitespace-pre-wrap">
                        {JSON.stringify(meta, null, 2)}
                      </pre>
                    </div>
                  </div>
                )}

                <div>
                  <label className="block text-sm font-medium text-gray-700 mb-1">
                    Content Preview
                  </label>
                  <div className="p-3 bg-gray-50 border border-gray-200 rounded-md max-h-32 overflow-y-auto">
                    <p className="text-xs text-gray-700 whitespace-pre-wrap">
                      {contentMd.substring(0, 500)}
                      {contentMd.length > 500 && '...'}
                    </p>
                  </div>
                </div>

                {error && (
                  <div className="bg-red-50 border border-red-200 text-red-600 px-4 py-2 rounded text-sm">
                    {error}
                  </div>
                )}
              </div>
            )}

            <div className="mt-6 flex gap-3">
              <button
                type="button"
                onClick={onClose}
                className="flex-1 px-4 py-2 border border-gray-300 text-gray-700 rounded-md hover:bg-gray-50"
              >
                Cancel
              </button>
              <button
                type="submit"
                disabled={loading || !file}
                className="flex-1 px-4 py-2 bg-blue-600 text-white rounded-md hover:bg-blue-700 disabled:opacity-50 disabled:cursor-not-allowed flex items-center justify-center gap-2"
              >
                {loading && (
                  <div className="animate-spin rounded-full h-4 w-4 border-2 border-white border-t-transparent"></div>
                )}
                {loading ? 'Uploading...' : 'Upload Document'}
              </button>
            </div>
          </form>
        </div>

        {/* Loading Overlay */}
        {loading && (
          <div className="absolute inset-0 bg-white bg-opacity-75 flex items-center justify-center rounded-lg">
            <div className="text-center">
              <div className="animate-spin rounded-full h-12 w-12 border-4 border-blue-600 border-t-transparent mx-auto mb-4"></div>
              <p className="text-gray-700 font-medium">Uploading document...</p>
              <p className="text-xs text-gray-500 mt-1">This may take a few seconds</p>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
