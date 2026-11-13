import React, { useState, useEffect } from 'react';
import { Search, Layers, AlertCircle, Cpu, BarChart2, Plus, Play, RefreshCw, X } from 'lucide-react';
import type { Queue } from '../api/client';

interface CommandPaletteProps {
  isOpen: boolean;
  onClose: () => void;
  queues: Queue[];
  onSelectQueue: (queue: Queue) => void;
  onNavigateTab: (tab: 'queues' | 'explorer' | 'workers' | 'dlq' | 'metrics') => void;
  onOpenNewJob: () => void;
  onOpenNewQueue: () => void;
  onReplayDLQ: () => void;
}

export const CommandPalette: React.FC<CommandPaletteProps> = ({
  isOpen,
  onClose,
  queues,
  onSelectQueue,
  onNavigateTab,
  onOpenNewJob,
  onOpenNewQueue,
  onReplayDLQ
}) => {
  const [query, setQuery] = useState('');
  const [selectedIndex, setSelectedIndex] = useState(0);

  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') {
        e.preventDefault();
        if (isOpen) {
          onClose();
        }
      } else if (e.key === 'Escape' && isOpen) {
        onClose();
      }
    };
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [isOpen, onClose]);

  if (!isOpen) return null;

  // Build searchable items
  const baseActions = [
    { id: 'tab-queues', title: 'Go to Queues Overview', icon: Layers, category: 'Navigation', action: () => onNavigateTab('queues') },
    { id: 'tab-explorer', title: 'Open Job Explorer', icon: Search, category: 'Navigation', action: () => onNavigateTab('explorer') },
    { id: 'tab-dlq', title: 'Open Dead Letter Queue (DLQ)', icon: AlertCircle, category: 'Navigation', action: () => onNavigateTab('dlq') },
    { id: 'tab-workers', title: 'View Worker Fleet', icon: Cpu, category: 'Navigation', action: () => onNavigateTab('workers') },
    { id: 'tab-metrics', title: 'View System Analytics & Throughput', icon: BarChart2, category: 'Navigation', action: () => onNavigateTab('metrics') },
    { id: 'action-new-job', title: 'Submit New Background Job', icon: Plus, category: 'Actions', action: onOpenNewJob },
    { id: 'action-new-queue', title: 'Create New Queue', icon: Play, category: 'Actions', action: onOpenNewQueue },
    { id: 'action-replay-dlq', title: 'Trigger DLQ Replay All', icon: RefreshCw, category: 'Actions', action: onReplayDLQ },
  ];

  const queueActions = queues.map((q) => ({
    id: `queue-${q.id}`,
    title: `Queue: ${q.name} (${q.state || 'active'})`,
    icon: Layers,
    category: 'Queues',
    action: () => {
      onSelectQueue(q);
      onNavigateTab('queues');
    }
  }));

  const allItems = [...baseActions, ...queueActions];
  const filtered = allItems.filter(item => 
    item.title.toLowerCase().includes(query.toLowerCase()) || 
    item.category.toLowerCase().includes(query.toLowerCase())
  );

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'ArrowDown') {
      e.preventDefault();
      setSelectedIndex(prev => (prev + 1) % (filtered.length || 1));
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      setSelectedIndex(prev => (prev - 1 + (filtered.length || 1)) % (filtered.length || 1));
    } else if (e.key === 'Enter' && filtered[selectedIndex]) {
      e.preventDefault();
      filtered[selectedIndex].action();
      onClose();
    }
  };

  return (
    <div className="cmd-palette-backdrop" onClick={onClose}>
      <div className="cmd-palette-card" onClick={e => e.stopPropagation()} onKeyDown={handleKeyDown}>
        <div className="cmd-input-wrapper">
          <Search size={18} color="var(--accent-cyan)" />
          <input
            autoFocus
            type="text"
            className="cmd-input"
            placeholder="Type a command, queue name, or action..."
            value={query}
            onChange={e => {
              setQuery(e.target.value);
              setSelectedIndex(0);
            }}
          />
          <button onClick={onClose} style={{ background: 'transparent', border: 'none', color: 'var(--text-muted)', cursor: 'pointer' }}>
            <X size={16} />
          </button>
        </div>

        <div className="cmd-list">
          {filtered.length === 0 ? (
            <div style={{ padding: '1.5rem', textAlign: 'center', color: 'var(--text-muted)', fontSize: '0.9rem' }}>
              No commands or queues matching &ldquo;{query}&rdquo;
            </div>
          ) : (
            filtered.map((item, idx) => {
              const Icon = item.icon;
              const isSelected = idx === selectedIndex;
              return (
                <div
                  key={item.id}
                  className={`cmd-item ${isSelected ? 'selected' : ''}`}
                  onClick={() => {
                    item.action();
                    onClose();
                  }}
                  onMouseEnter={() => setSelectedIndex(idx)}
                >
                  <Icon size={16} color={isSelected ? '#38bdf8' : 'var(--text-muted)'} />
                  <span style={{ flex: 1 }}>{item.title}</span>
                  <span style={{ fontSize: '0.72rem', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)' }}>
                    {item.category}
                  </span>
                </div>
              );
            })
          )}
        </div>

        <div style={{
          display: 'flex',
          justifyContent: 'space-between',
          padding: '0.6rem 1rem',
          borderTop: '1px solid var(--border-subtle)',
          fontSize: '0.75rem',
          color: 'var(--text-muted)',
          fontFamily: 'var(--font-mono)'
        }}>
          <span>Navigate with <kbd className="kbd-shortcut">&uarr;</kbd> <kbd className="kbd-shortcut">&darr;</kbd></span>
          <span>Select with <kbd className="kbd-shortcut">&crarr;</kbd></span>
        </div>
      </div>
    </div>
  );
};

