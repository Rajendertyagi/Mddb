import { Search, AlertCircle, Tag } from 'lucide-react';
import { useStore } from '../lib/store';
import mddbClient from '../lib/mddb-client';

export default function FTSSearchPanel() {
  const {
    currentCollection,
    ftsQuery, setFtsQuery,
    ftsLimit, setFtsLimit,
    ftsAlgorithm, setFtsAlgorithm,
    ftsFuzzy, setFtsFuzzy,
    ftsStemming, setFtsStemming,
    ftsSynonyms, setFtsSynonyms,
    ftsResults, setFtsResults,
    ftsLoading, setFtsLoading,
    ftsError, setFtsError,
    setCurrentDocument,
  } = useStore();

  const handleSearch = async () => {
    if (!currentCollection || !ftsQuery.trim()) return;

    setFtsLoading(true);
    setFtsError(null);
    try {
      const data = await mddbClient.ftsSearch({
        collection: currentCollection,
        query: ftsQuery.trim(),
        limit: ftsLimit,
        algorithm: ftsAlgorithm,
        fuzzy: ftsFuzzy,
        disableStem: !ftsStemming,
        disableSynonyms: !ftsSynonyms,
      });
      setFtsResults(data.results || []);
    } catch (error) {
      setFtsError(error.message);
      setFtsResults([]);
    } finally {
      setFtsLoading(false);
    }
  };

  const handleResultClick = async (result) => {
    const doc = result.document;
    if (!doc) return;

    const initialDocument = {
      ...doc,
      collection: currentCollection,
      contentMd: doc.contentMd || 'Loading content...',
    };
    setCurrentDocument(initialDocument);

    try {
      const fullDocument = await mddbClient.getDocument({
        collection: currentCollection,
        key: doc.key,
        lang: doc.lang,
      });
      setCurrentDocument({ ...fullDocument, collection: currentCollection });
    } catch (error) {
      setCurrentDocument({
        ...initialDocument,
        contentMd: `Error loading content: ${error.message}`,
      });
    }
  };

  if (!currentCollection) {
    return (
      <div className="flex items-center justify-center h-full">
        <div className="text-center">
          <Search className="w-12 h-12 text-gray-300 mx-auto mb-3" />
          <p className="text-gray-500">Select a collection to search</p>
        </div>
      </div>
    );
  }

  // Normalize score for bar width (max score = 1.0 for display)
  const maxScore = ftsResults.length > 0 ? Math.max(...ftsResults.map(r => r.score)) : 1;

  return (
    <div className="h-full flex flex-col">
      {/* Search Controls */}
      <div className="p-4 border-b border-gray-200 space-y-4">
        <div>
          <label className="block text-xs font-semibold text-gray-500 uppercase tracking-wider mb-1">
            Full-Text Query
          </label>
          <input
            type="text"
            value={ftsQuery}
            onChange={(e) => setFtsQuery(e.target.value)}
            onKeyDown={(e) => e.key === 'Enter' && handleSearch()}
            placeholder="Search documents by text..."
            className="w-full px-3 py-2 border border-gray-300 rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-primary-500"
          />
        </div>

        <div className="grid grid-cols-3 gap-3">
          <div>
            <label className="block text-xs font-medium text-gray-600 mb-1">Algorithm</label>
            <select
              value={ftsAlgorithm}
              onChange={(e) => setFtsAlgorithm(e.target.value)}
              className="w-full px-2 py-1.5 border border-gray-300 rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-primary-500"
            >
              <option value="tfidf">TF-IDF</option>
              <option value="bm25">BM25</option>
            </select>
          </div>
          <div>
            <label className="block text-xs font-medium text-gray-600 mb-1">Typo Tolerance</label>
            <select
              value={ftsFuzzy}
              onChange={(e) => setFtsFuzzy(parseInt(e.target.value))}
              className="w-full px-2 py-1.5 border border-gray-300 rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-primary-500"
            >
              <option value={0}>Off</option>
              <option value={1}>Low (1 edit)</option>
              <option value={2}>Medium (2 edits)</option>
            </select>
          </div>
          <div>
            <label className="block text-xs font-medium text-gray-600 mb-1">
              Limit: {ftsLimit}
            </label>
            <input
              type="range"
              min={1}
              max={200}
              value={ftsLimit}
              onChange={(e) => setFtsLimit(parseInt(e.target.value))}
              className="w-full accent-primary-600"
            />
          </div>
        </div>

        <div className="flex items-center justify-between">
          <div className="flex items-center space-x-4">
            <label className="flex items-center space-x-1.5 text-sm text-gray-600">
              <input
                type="checkbox"
                checked={ftsStemming}
                onChange={(e) => setFtsStemming(e.target.checked)}
                className="rounded border-gray-300 text-primary-600 focus:ring-primary-500"
              />
              <span>Stemming</span>
            </label>
            <label className="flex items-center space-x-1.5 text-sm text-gray-600">
              <input
                type="checkbox"
                checked={ftsSynonyms}
                onChange={(e) => setFtsSynonyms(e.target.checked)}
                className="rounded border-gray-300 text-primary-600 focus:ring-primary-500"
              />
              <span>Synonyms</span>
            </label>
          </div>
          <button
            onClick={handleSearch}
            disabled={ftsLoading || !ftsQuery.trim()}
            className="flex items-center space-x-2 px-4 py-2 bg-primary-600 text-white rounded-lg hover:bg-primary-700 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {ftsLoading ? (
              <div className="animate-spin rounded-full h-4 w-4 border-b-2 border-white" />
            ) : (
              <Search className="w-4 h-4" />
            )}
            <span className="text-sm font-medium">Search</span>
          </button>
        </div>
      </div>

      {/* Results */}
      <div className="flex-1 overflow-y-auto">
        {ftsResults.length > 0 && (
          <div className="px-4 pt-3 pb-1">
            <span className="text-xs font-medium text-gray-500">
              {ftsResults.length} result{ftsResults.length !== 1 ? 's' : ''} found
            </span>
          </div>
        )}

        {ftsError && (
          <div className="m-4 p-3 bg-red-50 border border-red-200 rounded-lg flex items-start space-x-2">
            <AlertCircle className="w-4 h-4 text-red-500 mt-0.5 flex-shrink-0" />
            <p className="text-sm text-red-700">{ftsError}</p>
          </div>
        )}

        {ftsResults.length === 0 && !ftsLoading && !ftsError && ftsQuery && (
          <div className="flex items-center justify-center h-32">
            <p className="text-gray-400 text-sm">No results found</p>
          </div>
        )}

        {ftsResults.length > 0 && (
          <div className="divide-y divide-gray-200">
            {ftsResults.map((result, idx) => {
              const doc = result.document;
              const pct = maxScore > 0 ? Math.round((result.score / maxScore) * 100) : 0;
              return (
                <button
                  key={`${doc?.key}-${doc?.lang}-${idx}`}
                  onClick={() => handleResultClick(result)}
                  className="w-full text-left p-4 hover:bg-gray-50 transition-colors"
                >
                  <div className="flex items-center justify-between mb-2">
                    <div className="flex items-center space-x-2">
                      <span className="inline-flex items-center justify-center w-6 h-6 rounded-full bg-primary-100 text-primary-700 text-xs font-bold">
                        {idx + 1}
                      </span>
                      <h4 className="font-medium text-gray-900 truncate">
                        {doc?.key}
                      </h4>
                      <span className="text-xs text-gray-500">{doc?.lang}</span>
                    </div>
                    <span className="text-sm font-semibold text-primary-600">
                      {result.score.toFixed(4)}
                    </span>
                  </div>

                  {/* Score bar */}
                  <div className="w-full bg-gray-200 rounded-full h-1.5 mb-2">
                    <div
                      className="bg-primary-500 h-1.5 rounded-full transition-all"
                      style={{ width: `${pct}%` }}
                    />
                  </div>

                  {/* Matched terms */}
                  {result.matchedTerms && result.matchedTerms.length > 0 && (
                    <div className="flex flex-wrap gap-1 mt-1">
                      {result.matchedTerms.map((term) => (
                        <span
                          key={term}
                          className="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium bg-yellow-100 text-yellow-800"
                        >
                          <Tag className="w-3 h-3 mr-1" />
                          {term}
                        </span>
                      ))}
                    </div>
                  )}

                  {/* Meta tags */}
                  {doc?.meta && Object.keys(doc.meta).length > 0 && (
                    <div className="flex flex-wrap gap-1 mt-1">
                      {Object.entries(doc.meta).slice(0, 3).map(([key, values]) => (
                        <span
                          key={key}
                          className="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium bg-gray-100 text-gray-700"
                        >
                          {key}: {Array.isArray(values) ? values.join(', ') : values}
                        </span>
                      ))}
                    </div>
                  )}
                </button>
              );
            })}
          </div>
        )}
      </div>
    </div>
  );
}
