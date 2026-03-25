/**
 * SSE (Server-Sent Events) hook for real-time document change notifications.
 * Connects to /v1/events and dispatches events to registered listeners.
 */
import { useEffect, useRef, useState, useCallback } from 'react';
import { authManager } from './auth';

const SSE_RECONNECT_DELAY = 3000; // 3s
const SSE_MAX_RECONNECT_DELAY = 30000; // 30s

/**
 * useSSE - React hook for SSE connection.
 * @param {Object} options
 * @param {string} [options.collection] - Filter events to a specific collection
 * @param {function} [options.onEvent] - Callback for each event: (event) => void
 * @param {boolean} [options.enabled=true] - Enable/disable SSE connection
 * @returns {{ connected: boolean, mode: string, lastEvent: object|null }}
 */
export function useSSE({ collection = '', onEvent, enabled = true } = {}) {
  const [connected, setConnected] = useState(false);
  const [mode, setMode] = useState('read');
  const [lastEvent, setLastEvent] = useState(null);
  const onEventRef = useRef(onEvent);
  const reconnectDelay = useRef(SSE_RECONNECT_DELAY);
  const reconnectTimer = useRef(null);
  const abortRef = useRef(null);

  // Keep callback ref current
  useEffect(() => {
    onEventRef.current = onEvent;
  }, [onEvent]);

  const connect = useCallback(() => {
    if (!enabled) return;

    // Build URL
    const params = new URLSearchParams();
    if (collection) params.set('collection', collection);
    const url = `/v1/events${params.toString() ? '?' + params.toString() : ''}`;

    // Use fetch for auth header support (EventSource doesn't support custom headers)
    const token = authManager.getToken();
    const headers = {};
    if (token) {
      headers['Authorization'] = `Bearer ${token}`;
    }

    const controller = new AbortController();
    abortRef.current = controller;

    fetch(url, { headers, signal: controller.signal })
      .then(async (response) => {
        if (!response.ok) {
          throw new Error(`SSE ${response.status}`);
        }

        setConnected(true);
        reconnectDelay.current = SSE_RECONNECT_DELAY;

        const reader = response.body.getReader();
        const decoder = new TextDecoder();
        let buffer = '';

        while (true) {
          const { done, value } = await reader.read();
          if (done) break;

          buffer += decoder.decode(value, { stream: true });
          const blocks = buffer.split('\n\n');
          buffer = blocks.pop() || '';

          for (const block of blocks) {
            if (!block.trim()) continue;

            const lines = block.split('\n');
            let eventType = '';
            let data = '';

            for (const line of lines) {
              if (line.startsWith('event: ')) eventType = line.slice(7);
              else if (line.startsWith('data: ')) data = line.slice(6);
              // Skip comments (: keepalive ...)
            }

            if (eventType === 'connected' && data) {
              try {
                const parsed = JSON.parse(data);
                setMode(parsed.mode || 'read');
              } catch { /* ignore */ }
              continue;
            }

            if (eventType && data) {
              try {
                const parsed = JSON.parse(data);
                setLastEvent(parsed);
                if (onEventRef.current) {
                  onEventRef.current(parsed);
                }
              } catch { /* ignore malformed */ }
            }
          }
        }
      })
      .catch((err) => {
        if (err.name === 'AbortError') return;
        console.warn('SSE connection lost:', err.message);
      })
      .finally(() => {
        setConnected(false);
        if (enabled && !controller.signal.aborted) {
          // Reconnect with exponential backoff
          reconnectTimer.current = setTimeout(() => {
            reconnectDelay.current = Math.min(reconnectDelay.current * 1.5, SSE_MAX_RECONNECT_DELAY);
            connect();
          }, reconnectDelay.current);
        }
      });
  }, [collection, enabled]);

  useEffect(() => {
    connect();
    return () => {
      if (abortRef.current) abortRef.current.abort();
      if (reconnectTimer.current) clearTimeout(reconnectTimer.current);
    };
  }, [connect]);

  return { connected, mode, lastEvent };
}
