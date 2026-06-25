import { useState, useEffect, useRef } from 'react';
import type { ResourceGraph, SSEStatus } from '../api/types';

export default function useSSE(onGraph: (data: ResourceGraph) => void) {
  const [sseStatus, setSseStatus] = useState<SSEStatus>('disconnected');
  const onGraphRef = useRef(onGraph);
  onGraphRef.current = onGraph;

  useEffect(() => {
    let es: EventSource | null = null;
    let retryTimer: ReturnType<typeof setTimeout> | null = null;
    let delay = 1000;
    const maxDelay = 5000;
    let closed = false;

    function connect() {
      if (closed) return;
      es = new EventSource('/api/stream');

      es.onopen = () => {
        setSseStatus('connected');
        delay = 1000; // reset on success
      };

      es.onmessage = (event) => {
        try {
          const data = JSON.parse(event.data) as unknown;
          if (data && typeof data === 'object' && 'nodes' in data) {
            onGraphRef.current(data as ResourceGraph);
          }
        } catch {
          console.error('Failed to parse SSE graph stream');
        }
      };

      es.onerror = () => {
        es?.close();
        es = null;
        if (closed) return;
        setSseStatus('reconnecting');
        retryTimer = setTimeout(() => {
          delay = Math.min(delay * 2, maxDelay);
          connect();
        }, delay);
      };
    }

    connect();

    return () => {
      closed = true;
      if (retryTimer) clearTimeout(retryTimer);
      es?.close();
      es = null;
    };
  }, []);

  return sseStatus;
}
