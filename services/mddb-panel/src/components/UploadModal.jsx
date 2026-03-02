import { useState } from 'react';
import { X, FileText, Upload } from 'lucide-react';
import mddbClient from '../lib/mddb-client';

export default function UploadModal({ collection, onClose, onSuccess }) {
  const [files, setFiles] = useState([]);
  const [collectionName, setCollectionName] = useState(collection || '');
  const [lang, setLang] = useState('en');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(null);
  const [dragActive, setDragActive] = useState(false);
  const [uploadProgress, setUploadProgress] = useState({ current: 0, total: 0 });
  const [successCount, setSuccessCount] = useState(0);

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

  const handleFiles = async (selectedFiles) => {
    if (!selectedFiles || selectedFiles.length === 0) return;

    const fileArray = Array.from(selectedFiles);
    const validFiles = [];
    const errors = [];

    // Check file size (max 10MB per file)
    const maxSize = 10 * 1024 * 1024;

    for (const file of fileArray) {
      if (!file.name.endsWith('.md')) {
        errors.push(`${file.name}: Only .md files supported`);
        continue;
      }

      if (file.size > maxSize) {
        errors.push(`${file.name}: File too large (max 10MB)`);
        continue;
      }

      // Read and parse file
      try {
        const text = await readFileAsText(file);

        if (text.length > 5 * 1024 * 1024) {
          errors.push(`${file.name}: Content too large (max 5MB)`);
          continue;
        }

        const { metadata, content } = parseFrontmatter(text);
        const key = file.name.replace(/\.md$/, '');

        validFiles.push({
          file,
          key,
          meta: metadata,
          contentMd: content,
        });
      } catch (err) {
        errors.push(`${file.name}: Failed to read file`);
      }
    }

    if (errors.length > 0) {
      setError(errors.join('; '));
    } else {
      setError(null);
    }

    setFiles(validFiles);
  };

  const readFileAsText = (file) => {
    return new Promise((resolve, reject) => {
      const reader = new FileReader();
      reader.onload = (e) => resolve(e.target.result);
      reader.onerror = () => reject(new Error('Failed to read file'));
      reader.readAsText(file);
    });
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
      handleFiles(e.dataTransfer.files);
    }
  };

  const handleSubmit = async (e) => {
    e.preventDefault();
    setError(null);
    setLoading(true);
    setSuccessCount(0);
    setUploadProgress({ current: 0, total: files.length });

    const finalLang = (lang || '').trim() || 'en';
    const finalCollection = (collectionName || '').trim() || 'default';

    // Validate we have files to upload
    if (files.length === 0) {
      setError('No files selected. Please select one or more markdown files.');
      setLoading(false);
      return;
    }

    console.log(`Starting bulk upload of ${files.length} files to collection: ${finalCollection}`);

    const errors = [];
    let successfulUploads = 0;

    // Upload files one by one
    for (let i = 0; i < files.length; i++) {
      const fileData = files[i];
      setUploadProgress({ current: i + 1, total: files.length });

      try {
        const timeout = new Promise((_, reject) =>
          setTimeout(() => reject(new Error('Upload timeout (>30s)')), 30000)
        );

        await Promise.race([
          mddbClient.addDocument({
            collection: finalCollection,
            key: fileData.key,
            lang: finalLang,
            meta: fileData.meta,
            contentMd: fileData.contentMd,
          }),
          timeout
        ]);

        successfulUploads++;
        setSuccessCount(successfulUploads);
        console.log(`✓ Uploaded ${fileData.key} (${i + 1}/${files.length})`);
      } catch (err) {
        console.error(`✗ Failed to upload ${fileData.key}:`, err);
        errors.push(`${fileData.key}: ${err.message}`);
      }
    }

    setLoading(false);

    if (errors.length > 0) {
      setError(`Uploaded ${successfulUploads}/${files.length} files. Errors: ${errors.join('; ')}`);
      // Still call onSuccess if at least some files uploaded
      if (successfulUploads > 0) {
        setTimeout(() => onSuccess(finalCollection), 2000);
      }
    } else {
      console.log(`✓ All ${files.length} files uploaded successfully!`);
      onSuccess(finalCollection);
    }
  };

  return (
    <div className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50 p-4">
      <div className="bg-white rounded-lg shadow-xl w-full max-w-2xl max-h-[90vh] overflow-y-auto">
        <div className="p-6">
          <div className="flex items-center justify-between mb-4">
            <h2 className="text-xl font-bold text-gray-900">
              {collection ? 'Upload Document' : 'Create Collection & Upload Document'}
            </h2>
            <button onClick={onClose} className="text-gray-400 hover:text-gray-600" disabled={loading}>
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
                    className="mt-2 text-sm text-blue-600 hover:text-blue-700"
                    disabled={loading}
                  >
                    Choose different file
                  </button>
                </div>
              ) : (
                <div>
                  <Upload className="w-12 h-12 text-gray-400 mx-auto mb-2" />
                  <p className="text-sm font-medium text-gray-900 mb-1">
                    Drop your markdown file here
                  </p>
                  <p className="text-xs text-gray-500 mb-3">or</p>
                  <label className="inline-block px-4 py-2 bg-blue-600 text-white rounded-lg hover:bg-blue-700 cursor-pointer transition-colors">
                    Browse Files
                    <input
                      type="file"
                      accept=".md"
                      onChange={(e) => handleFile(e.target.files[0])}
                      className="hidden"
                    />
                  </label>
                  <p className="text-xs text-gray-500 mt-3">
                    Max file size: 10MB
                  </p>
                </div>
              )}
            </div>

            {/* Show parsed metadata */}
            {file && Object.keys(meta).length > 0 && (
              <div className="mb-4 p-4 bg-gray-50 rounded-lg">
                <p className="text-sm font-medium text-gray-700 mb-2">
                  Parsed Metadata ({Object.keys(meta).length} fields)
                </p>
                <div className="space-y-1 max-h-40 overflow-y-auto">
                  {Object.entries(meta).map(([key, values]) => (
                    <div key={key} className="text-xs text-gray-600">
                      <span className="font-medium">{key}:</span>{' '}
                      {Array.isArray(values) ? values.join(', ') : values}
                    </div>
                  ))}
                </div>
              </div>
            )}

            {/* Collection input */}
            <div className="mb-4">
              <label className="block text-sm font-medium text-gray-700 mb-1">
                Collection {!collection && <span className="text-red-600">*</span>}
              </label>
              <input
                type="text"
                value={collectionName}
                onChange={(e) => setCollectionName(e.target.value)}
                placeholder="Enter collection name"
                className="w-full px-3 py-2 border border-gray-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-blue-500"
                disabled={loading || !!collection}
              />
              <p className="text-xs text-gray-500 mt-1">
                {collection
                  ? `Uploading to existing collection: ${collection}`
                  : 'New collection will be created if it doesn\'t exist'
                }
              </p>
            </div>

            {/* Key input */}
            <div className="mb-4">
              <label className="block text-sm font-medium text-gray-700 mb-1">
                Document Key
              </label>
              <input
                type="text"
                value={key}
                onChange={(e) => setKey(e.target.value)}
                placeholder="Auto-generated from filename"
                className="w-full px-3 py-2 border border-gray-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-blue-500"
                disabled={loading}
              />
              <p className="text-xs text-gray-500 mt-1">
                Leave empty for auto-generated key
              </p>
            </div>

            {/* Language input */}
            <div className="mb-4">
              <label className="block text-sm font-medium text-gray-700 mb-1">
                Language
              </label>
              <input
                type="text"
                value={lang}
                onChange={(e) => setLang(e.target.value)}
                placeholder="en"
                className="w-full px-3 py-2 border border-gray-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-blue-500"
                disabled={loading}
              />
            </div>

            {/* Error display */}
            {error && (
              <div className="mb-4 p-3 bg-red-50 border border-red-200 rounded-lg">
                <p className="text-sm text-red-800">{error}</p>
              </div>
            )}

            {/* Buttons */}
            <div className="flex justify-end gap-3">
              <button
                type="button"
                onClick={onClose}
                className="px-4 py-2 text-gray-700 bg-gray-100 hover:bg-gray-200 rounded-lg transition-colors"
                disabled={loading}
              >
                Cancel
              </button>
              <button
                type="submit"
                disabled={!file || loading}
                className="px-4 py-2 bg-blue-600 text-white rounded-lg hover:bg-blue-700 disabled:opacity-50 disabled:cursor-not-allowed transition-colors flex items-center gap-2"
              >
                {loading ? (
                  <>
                    <div className="animate-spin rounded-full h-4 w-4 border-2 border-white border-t-transparent"></div>
                    Uploading...
                  </>
                ) : (
                  <>
                    <Upload className="w-4 h-4" />
                    Upload Document
                  </>
                )}
              </button>
            </div>
          </form>
        </div>
      </div>

      {/* Loading overlay */}
      {loading && (
        <div className="absolute inset-0 bg-white bg-opacity-75 flex items-center justify-center rounded-lg">
          <div className="text-center">
            <div className="animate-spin rounded-full h-12 w-12 border-4 border-blue-600 border-t-transparent mx-auto mb-4"></div>
            <p className="text-sm text-gray-600">Uploading document...</p>
            <p className="text-xs text-gray-500 mt-1">This may take a few seconds</p>
          </div>
        </div>
      )}
    </div>
  );
}
