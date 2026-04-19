import { useState, useRef, useEffect } from 'react';
import { Search, AlertCircle, Tag, ChevronDown, ChevronUp, Plus, X, Terminal, Ban, RotateCcw } from 'lucide-react';
import { useStore } from '../lib/store';
import mddbClient from '../lib/mddb-client';
import CommandModal from './CommandModal';
import MetaFilterBar from './MetaFilterBar';
import SpellSuggestionBadge from './SpellSuggestionBadge';

export default function FTSSearchPanel() {
  const {
    currentCollection,
    ftsQuery, setFtsQuery,
    ftsLimit, setFtsLimit,
    ftsAlgorithm, setFtsAlgorithm,
    ftsFuzzy, setFtsFuzzy,
    ftsLang, setFtsLang,
    ftsMode, setFtsMode,
    ftsDistance, setFtsDistance,
    ftsStemming, setFtsStemming,
    ftsSynonyms, setFtsSynonyms,
    ftsHighlight, setFtsHighlight,
    ftsFieldWeights, setFtsFieldWeight, removeFtsFieldWeight,
    ftsBoost, setFtsBoostEntry, removeFtsBoostEntry,
    ftsResults, setFtsResults,
    ftsLoading, setFtsLoading,
    ftsError, setFtsError,
    ftsSearchStats, setFtsSearchStats,
    searchFilterMeta,
    setCurrentDocument,
  } = useStore();

  const [weightsOpen, setWeightsOpen] = useState(true);
  const [newFieldName, setNewFieldName] = useState('');
  const [boostOpen, setBoostOpen] = useState(false);
  const [newBoostKey, setNewBoostKey] = useState('');
  const [newBoostValue, setNewBoostValue] = useState('1');
  const [suggestions, setSuggestions] = useState([]);
  const [suggestionsOpen, setSuggestionsOpen] = useState(false);
  const suggestionsAbortRef = useRef(null);
  const suggestionsTimerRef = useRef(null);
  const [showCommand, setShowCommand] = useState(false);
  const [spellCorrected, setSpellCorrected] = useState(null);
  const [availableLangs, setAvailableLangs] = useState([]);
  const [reindexing, setReindexing] = useState(false);
  const [reindexResult, setReindexResult] = useState(null);
  // (v2.9.14+) Facets: comma-separated list of meta keys the server aggregates into per-value counts.
  const [facetBy, setFacetBy] = useState('');
  const [facets, setFacets] = useState(null);
  const abortRef = useRef(null);

  useEffect(() => {
    mddbClient.ftsLanguages().then((data) => {
      setAvailableLangs(data.languages || []);
    }).catch(() => { });
  }, []);

  const handleCancel = () => {
    if (abortRef.current) {
      abortRef.current.abort();
      abortRef.current = null;
    }
  };

  // Debounced autocomplete: cancels any in-flight request + scheduled timer
  // so bursts of keystrokes only produce one network round-trip.
  const requestSuggestions = (query) => {
    if (suggestionsTimerRef.current) {
      clearTimeout(suggestionsTimerRef.current);
    }
    if (suggestionsAbortRef.current) {
      suggestionsAbortRef.current.abort();
      suggestionsAbortRef.current = null;
    }
    if (!currentCollection || !query || query.length < 2) {
      setSuggestions([]);
      setSuggestionsOpen(false);
      return;
    }
    suggestionsTimerRef.current = setTimeout(async () => {
      const controller = new AbortController();
      suggestionsAbortRef.current = controller;
      try {
        const data = await mddbClient.autocomplete({
          collection: currentCollection,
          q: query,
          topN: 8,
          signal: controller.signal,
        });
        if (!controller.signal.aborted) {
          setSuggestions(data.items || []);
          setSuggestionsOpen((data.items || []).length > 0);
        }
      } catch (e) {
        if (e.name !== 'AbortError') {
          setSuggestions([]);
          setSuggestionsOpen(false);
        }
      }
    }, 150);
  };

  const handleQueryChange = (value) => {
    setFtsQuery(value);
    requestSuggestions(value);
  };

  const acceptSuggestion = (term) => {
    setFtsQuery(term);
    setSuggestions([]);
    setSuggestionsOpen(false);
  };

  const handleSearch = async () => {
    if (!currentCollection || !ftsQuery.trim()) return;

    handleCancel();
    const controller = new AbortController();
    abortRef.current = controller;

    setFtsLoading(true);
    setFtsError(null);
    try {
      const data = await mddbClient.ftsSearch({
        collection: currentCollection,
        query: ftsQuery.trim(),
        limit: ftsLimit,
        algorithm: ftsAlgorithm,
        fuzzy: ftsFuzzy,
        mode: ftsMode,
        distance: ftsMode === 'proximity' ? ftsDistance : undefined,
        disableStem: !ftsStemming,
        disableSynonyms: !ftsSynonyms,
        fieldWeights: ftsAlgorithm === 'bm25f' ? ftsFieldWeights : null,
        filterMeta: searchFilterMeta,
        lang: ftsLang || undefined,
        boost: ftsBoost,
        highlight: ftsHighlight,
        facetBy: facetBy
          ? facetBy.split(',').map((s) => s.trim()).filter(Boolean)
          : undefined,
        signal: controller.signal,
      });
      setFtsResults(data.results || []);
      setFtsSearchStats(data.searchStats || null);
      setSpellCorrected(data.spellCorrected || null);
      setFacets(data.facets || null);
    } catch (error) {
      if (error.name === 'AbortError') {
        setFtsError(null);
      } else {
        setFtsError(error.message);
        setFtsResults([]);
        setFtsSearchStats(null);
      }
    } finally {
      setFtsLoading(false);
      abortRef.current = null;
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

  const handleAddField = () => {
    const name = newFieldName.trim();
    if (name && !(name in ftsFieldWeights)) {
      setFtsFieldWeight(name, 1.0);
      setNewFieldName('');
    }
  };

  const handleAddBoost = () => {
    const key = newBoostKey.trim();
    const value = parseFloat(newBoostValue);
    if (!key.includes(':') || key.startsWith(':') || key.endsWith(':')) return;
    if (Number.isNaN(value) || value === 0) return;
    setFtsBoostEntry(key, value);
    setNewBoostKey('');
    setNewBoostValue('1');
  };

  const handleFtsReindex = async () => {
    if (!currentCollection) return;
    setReindexing(true);
    setReindexResult(null);
    try {
      const result = await mddbClient.ftsReindex({ collection: currentCollection });
      setReindexResult(result);
    } catch (error) {
      setReindexResult({ error: error.message });
    } finally {
      setReindexing(false);
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
          <div className="relative">
            <input
              type="text"
              value={ftsQuery}
              onChange={(e) => handleQueryChange(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === 'Enter') {
                  setSuggestionsOpen(false);
                  handleSearch();
                } else if (e.key === 'Escape') {
                  setSuggestionsOpen(false);
                }
              }}
              onBlur={() => setTimeout(() => setSuggestionsOpen(false), 120)}
              onFocus={() => suggestions.length > 0 && setSuggestionsOpen(true)}
              placeholder="Search documents by text..."
              className="w-full px-3 py-2 border border-gray-300 rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-primary-500"
              autoComplete="off"
            />
            {suggestionsOpen && suggestions.length > 0 && (
              <ul className="absolute left-0 right-0 top-full mt-1 z-30 bg-white border border-gray-200 rounded-lg shadow-lg max-h-64 overflow-y-auto">
                {suggestions.map((s) => (
                  <li key={s.term}>
                    <button
                      type="button"
                      onMouseDown={(e) => { e.preventDefault(); acceptSuggestion(s.term); }}
                      className="w-full text-left px-3 py-1.5 text-sm hover:bg-primary-50 flex items-center justify-between"
                    >
                      <span className="truncate">{s.term}</span>
                      <span className="text-[11px] text-gray-400 ml-2">{s.docCount} docs</span>
                    </button>
                  </li>
                ))}
              </ul>
            )}
          </div>
        </div>

        <div className="grid grid-cols-5 gap-3">
          <div>
            <label className="block text-xs font-medium text-gray-600 mb-1">Language</label>
            <select
              value={ftsLang}
              onChange={(e) => setFtsLang(e.target.value)}
              className="w-full px-2 py-1.5 border border-gray-300 rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-primary-500"
            >
              <option value="">Auto (default)</option>
              {availableLangs.map((l) => (
                <option key={l.code} value={l.code}>{l.name} ({l.code})</option>
              ))}
            </select>
          </div>
          <div>
            <label className="block text-xs font-medium text-gray-600 mb-1">Search Mode</label>
            <select
              value={ftsMode}
              onChange={(e) => setFtsMode(e.target.value)}
              className="w-full px-2 py-1.5 border border-gray-300 rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-primary-500"
            >
              <option value="auto">Auto-detect</option>
              <option value="simple">Simple</option>
              <option value="boolean">Boolean (AND/OR/NOT)</option>
              <option value="phrase">Phrase (exact)</option>
              <option value="wildcard">Wildcard (*/?) </option>
              <option value="proximity">Proximity (~N)</option>
              <option value="expression">Expression (parens, precedence, mixed atoms)</option>
            </select>
          </div>
          <div>
            <label className="block text-xs font-medium text-gray-600 mb-1">Algorithm</label>
            <select
              value={ftsAlgorithm}
              onChange={(e) => setFtsAlgorithm(e.target.value)}
              className="w-full px-2 py-1.5 border border-gray-300 rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-primary-500"
              disabled={ftsMode !== 'simple' && ftsMode !== 'auto'}
            >
              <option value="tfidf">TF-IDF</option>
              <option value="bm25">BM25</option>
              <option value="bm25f">BM25F (Field-Weighted)</option>
              <option value="pmisparse">PMISparse (PMI Expansion)</option>
            </select>
          </div>
          <div>
            <label className="block text-xs font-medium text-gray-600 mb-1">Typo Tolerance</label>
            <select
              value={ftsFuzzy}
              onChange={(e) => setFtsFuzzy(parseInt(e.target.value))}
              className="w-full px-2 py-1.5 border border-gray-300 rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-primary-500"
              disabled={ftsMode !== 'simple' && ftsMode !== 'auto'}
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

        {/* Proximity Distance (shown when mode=proximity) */}
        {ftsMode === 'proximity' && (
          <div className="flex items-center space-x-3">
            <label className="text-xs font-medium text-gray-600 whitespace-nowrap">
              Max Distance: {ftsDistance} words
            </label>
            <input
              type="range"
              min={1}
              max={50}
              value={ftsDistance}
              onChange={(e) => setFtsDistance(parseInt(e.target.value))}
              className="flex-1 accent-primary-600"
            />
          </div>
        )}

        {/* Search mode hint */}
        {ftsMode !== 'simple' && ftsMode !== 'auto' && (
          <div className="text-xs text-gray-500 bg-gray-50 rounded-lg px-3 py-2">
            {ftsMode === 'boolean' && 'Use AND, OR, NOT operators: "rust AND performance", "+required -excluded"'}
            {ftsMode === 'phrase' && 'Enter exact phrase to match: "machine learning algorithms"'}
            {ftsMode === 'wildcard' && 'Use * (any chars) and ? (single char): prog*, te?t'}
            {ftsMode === 'proximity' && 'Enter terms to find within N words: "rust systems"'}
          </div>
        )}

        {/* BM25F Field Weights */}
        {ftsAlgorithm === 'bm25f' && (
          <div className="border border-gray-200 rounded-lg">
            <button
              onClick={() => setWeightsOpen(!weightsOpen)}
              className="w-full flex items-center justify-between px-3 py-2 text-xs font-semibold text-gray-600 uppercase tracking-wider hover:bg-gray-50"
            >
              <span>Field Weights</span>
              {weightsOpen ? <ChevronUp className="w-3.5 h-3.5" /> : <ChevronDown className="w-3.5 h-3.5" />}
            </button>
            {weightsOpen && (
              <div className="px-3 pb-3 space-y-2">
                {Object.entries(ftsFieldWeights).map(([field, weight]) => (
                  <div key={field} className="flex items-center space-x-2">
                    <span className="text-xs text-gray-600 w-28 truncate" title={field}>{field}</span>
                    <input
                      type="number"
                      min={0}
                      max={10}
                      step={0.5}
                      value={weight}
                      onChange={(e) => setFtsFieldWeight(field, parseFloat(e.target.value) || 0)}
                      className="w-20 px-2 py-1 border border-gray-300 rounded text-xs text-center focus:outline-none focus:ring-1 focus:ring-primary-500"
                    />
                    <button
                      onClick={() => removeFtsFieldWeight(field)}
                      className="p-0.5 text-gray-400 hover:text-red-500"
                      title="Remove field"
                    >
                      <X className="w-3.5 h-3.5" />
                    </button>
                  </div>
                ))}
                <div className="flex items-center space-x-2 pt-1">
                  <input
                    type="text"
                    value={newFieldName}
                    onChange={(e) => setNewFieldName(e.target.value)}
                    onKeyDown={(e) => e.key === 'Enter' && handleAddField()}
                    placeholder="meta.author"
                    className="flex-1 px-2 py-1 border border-gray-300 rounded text-xs focus:outline-none focus:ring-1 focus:ring-primary-500"
                  />
                  <button
                    onClick={handleAddField}
                    disabled={!newFieldName.trim()}
                    className="flex items-center space-x-1 px-2 py-1 text-xs text-primary-600 hover:bg-primary-50 rounded disabled:opacity-40"
                  >
                    <Plus className="w-3 h-3" />
                    <span>Add</span>
                  </button>
                </div>
              </div>
            )}
          </div>
        )}

        {/* Per-query boost map: metaKey:metaValue → multiplier */}
        <div className="border border-gray-200 rounded-lg">
          <button
            onClick={() => setBoostOpen(!boostOpen)}
            className="w-full flex items-center justify-between px-3 py-2 text-xs font-semibold text-gray-600 uppercase tracking-wider hover:bg-gray-50"
          >
            <span>
              Boost / Demote
              {Object.keys(ftsBoost).length > 0 && (
                <span className="ml-2 text-primary-600 normal-case text-[11px] font-normal">
                  {Object.keys(ftsBoost).length} active
                </span>
              )}
            </span>
            {boostOpen ? <ChevronUp className="w-3.5 h-3.5" /> : <ChevronDown className="w-3.5 h-3.5" />}
          </button>
          {boostOpen && (
            <div className="px-3 pb-3 space-y-2">
              <p className="text-[11px] text-gray-500">
                Key format <code className="bg-gray-100 px-1 rounded">metaKey:metaValue</code>. Positive = boost (5.0 = 5×), negative = demote (-2.0 = ½×).
              </p>
              {Object.entries(ftsBoost).map(([key, value]) => (
                <div key={key} className="flex items-center space-x-2">
                  <span className="text-xs text-gray-600 flex-1 truncate" title={key}>{key}</span>
                  <input
                    type="number"
                    step={0.5}
                    value={value}
                    onChange={(e) => setFtsBoostEntry(key, parseFloat(e.target.value) || 0)}
                    className="w-20 px-2 py-1 border border-gray-300 rounded text-xs text-center focus:outline-none focus:ring-1 focus:ring-primary-500"
                  />
                  <button
                    onClick={() => removeFtsBoostEntry(key)}
                    className="p-0.5 text-gray-400 hover:text-red-500"
                    title="Remove boost entry"
                  >
                    <X className="w-3.5 h-3.5" />
                  </button>
                </div>
              ))}
              <div className="flex items-center space-x-2 pt-1">
                <input
                  type="text"
                  value={newBoostKey}
                  onChange={(e) => setNewBoostKey(e.target.value)}
                  onKeyDown={(e) => e.key === 'Enter' && handleAddBoost()}
                  placeholder="tag:featured"
                  className="flex-1 px-2 py-1 border border-gray-300 rounded text-xs focus:outline-none focus:ring-1 focus:ring-primary-500"
                />
                <input
                  type="number"
                  step={0.5}
                  value={newBoostValue}
                  onChange={(e) => setNewBoostValue(e.target.value)}
                  onKeyDown={(e) => e.key === 'Enter' && handleAddBoost()}
                  className="w-20 px-2 py-1 border border-gray-300 rounded text-xs text-center focus:outline-none focus:ring-1 focus:ring-primary-500"
                />
                <button
                  onClick={handleAddBoost}
                  disabled={!newBoostKey.includes(':')}
                  className="flex items-center space-x-1 px-2 py-1 text-xs text-primary-600 hover:bg-primary-50 rounded disabled:opacity-40"
                >
                  <Plus className="w-3 h-3" />
                  <span>Add</span>
                </button>
              </div>
            </div>
          )}
        </div>

        <MetaFilterBar collection={currentCollection} />

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
            <label className="flex items-center space-x-1.5 text-sm text-gray-600" title="Return highlighted fragments with matched terms wrapped in <mark>">
              <input
                type="checkbox"
                checked={ftsHighlight}
                onChange={(e) => setFtsHighlight(e.target.checked)}
                className="rounded border-gray-300 text-primary-600 focus:ring-primary-500"
              />
              <span>Highlight</span>
            </label>
            <label className="flex items-center space-x-1.5 text-sm text-gray-600" title="(v2.9.14+) Comma-separated meta keys to aggregate into value counts alongside results.">
              <span>Facets:</span>
              <input
                type="text"
                value={facetBy}
                onChange={(e) => setFacetBy(e.target.value)}
                placeholder="category,lang"
                className="w-40 px-2 py-1 border border-gray-300 rounded text-xs focus:outline-none focus:ring-1 focus:ring-primary-500"
              />
            </label>
          </div>
          <div className="flex items-center space-x-2">
            <button
              onClick={() => setShowCommand(true)}
              className="flex items-center space-x-2 px-3 py-2 bg-gray-100 text-gray-700 rounded-lg hover:bg-gray-200 transition-colors"
            >
              <Terminal className="w-4 h-4" />
              <span className="text-sm font-medium">Command</span>
            </button>
            {ftsLoading ? (
              <button
                onClick={handleCancel}
                className="flex items-center space-x-2 px-4 py-2 bg-red-600 text-white rounded-lg hover:bg-red-700 transition-colors"
              >
                <Ban className="w-4 h-4" />
                <span className="text-sm font-medium">Cancel</span>
              </button>
            ) : (
              <button
                onClick={handleSearch}
                disabled={!ftsQuery.trim()}
                className="flex items-center space-x-2 px-4 py-2 bg-primary-600 text-white rounded-lg hover:bg-primary-700 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
              >
                <Search className="w-4 h-4" />
                <span className="text-sm font-medium">Search</span>
              </button>
            )}
          </div>
        </div>
      </div>

      {/* Results */}
      <div className="flex-1 overflow-y-auto">
        {spellCorrected && (
          <div className="px-4 pt-3">
            <SpellSuggestionBadge original={spellCorrected.original} corrected={spellCorrected.corrected} />
          </div>
        )}
        {facets && Object.keys(facets).length > 0 && (
          <div className="px-4 pt-3">
            <div className="bg-gray-50 border border-gray-200 rounded-lg p-3">
              <p className="text-xs font-semibold uppercase tracking-wide text-gray-500 mb-2">Facets</p>
              <div className="space-y-2">
                {Object.entries(facets).map(([key, buckets]) => (
                  <div key={key}>
                    <p className="text-xs font-medium text-gray-700 mb-1">{key}</p>
                    {buckets.length === 0 ? (
                      <p className="text-xs text-gray-400 italic">(no values)</p>
                    ) : (
                      <div className="flex flex-wrap gap-1">
                        {buckets.map((b) => (
                          <span key={b.value} className="inline-flex items-center gap-1 px-2 py-0.5 text-xs rounded-full bg-white border border-gray-300 text-gray-700">
                            <span className="font-mono">{b.value}</span>
                            <span className="text-gray-500">{b.count}</span>
                          </span>
                        ))}
                      </div>
                    )}
                  </div>
                ))}
              </div>
            </div>
          </div>
        )}
        {ftsResults.length > 0 && (
          <div className="px-4 pt-3 pb-1 flex items-center justify-between">
            <span className="text-xs font-medium text-gray-500">
              {ftsResults.length} result{ftsResults.length !== 1 ? 's' : ''} found
            </span>
            {ftsSearchStats && (
              <span className="text-xs text-gray-400">
                {ftsSearchStats.durationMs}ms | {ftsSearchStats.totalTokens} token{ftsSearchStats.totalTokens !== 1 ? 's' : ''}{ftsSearchStats.queryTerms?.length > 0 ? ` | ${ftsSearchStats.queryTerms.join(', ')}` : ''}
              </span>
            )}
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
                      {result.pinned && (
                        <span className="inline-flex items-center px-1.5 py-0.5 rounded text-[10px] font-semibold bg-amber-100 text-amber-800" title="Pinned by a curation rule">
                          PINNED
                        </span>
                      )}
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

                  {/* Highlight fragments (v2.9.14+) — server wraps matches in <mark>. */}
                  {result.highlights && result.highlights.length > 0 && (
                    <div className="mt-2 space-y-1">
                      {result.highlights.map((h, i) => (
                        <p
                          key={i}
                          className="text-xs text-gray-700 leading-relaxed bg-gray-50 rounded px-2 py-1"
                          dangerouslySetInnerHTML={{ __html: h.fragment }}
                        />
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

      {/* FTS Reindex Footer */}
      <div className="p-3 border-t border-gray-200">
        {reindexResult && (
          <div className={`mb-2 p-2 rounded text-xs ${reindexResult.error
              ? 'bg-red-50 text-red-700'
              : 'bg-green-50 text-green-700'
            }`}>
            {reindexResult.error
              ? reindexResult.error
              : `Reindexed: ${reindexResult.reindexed || 0}, Skipped: ${reindexResult.skipped || 0}`
            }
          </div>
        )}
        <button
          onClick={handleFtsReindex}
          disabled={reindexing}
          className="w-full flex items-center justify-center space-x-2 px-3 py-2 bg-gray-100 text-gray-700 rounded-lg hover:bg-gray-200 transition-colors disabled:opacity-50"
        >
          <RotateCcw className={`w-4 h-4 ${reindexing ? 'animate-spin' : ''}`} />
          <span className="text-sm font-medium">
            {reindexing ? 'Reindexing FTS...' : 'Reindex FTS'}
          </span>
        </button>
      </div>

      {/* Command Modal */}
      <CommandModal
        isOpen={showCommand}
        onClose={() => setShowCommand(false)}
        type="fts"
        params={{
          collection: currentCollection,
          query: ftsQuery,
          limit: ftsLimit,
          algorithm: ftsAlgorithm,
          fuzzy: ftsFuzzy,
          mode: ftsMode,
          distance: ftsMode === 'proximity' ? ftsDistance : undefined,
          disableStem: !ftsStemming,
          disableSynonyms: !ftsSynonyms,
          fieldWeights: ftsAlgorithm === 'bm25f' ? ftsFieldWeights : null,
          lang: ftsLang || undefined,
          filterMeta: searchFilterMeta,
          boost: ftsBoost,
        }}
      />
    </div>
  );
}
