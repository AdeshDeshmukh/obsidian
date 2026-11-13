import React, { useState } from 'react';
import { Copy, Check } from 'lucide-react';

interface JsonViewerProps {
  data: any;
  title?: string;
  defaultExpanded?: boolean;
}

export const JsonViewer: React.FC<JsonViewerProps> = ({ data, title }) => {
  const [copied, setCopied] = useState(false);

  const formatted = JSON.stringify(data, null, 2);

  const handleCopy = () => {
    navigator.clipboard.writeText(formatted);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  return (
    <div style={{ position: 'relative', borderRadius: '8px', overflow: 'hidden' }}>
      <div style={{
        display: 'flex',
        justifyContent: 'space-between',
        alignItems: 'center',
        padding: '0.45rem 0.75rem',
        background: 'rgba(255, 255, 255, 0.04)',
        borderBottom: '1px solid rgba(255, 255, 255, 0.06)',
        fontSize: '0.78rem',
        fontFamily: 'var(--font-mono)',
        color: 'var(--text-secondary)'
      }}>
        <span>{title || 'JSON PAYLOAD'}</span>
        <button
          onClick={handleCopy}
          style={{
            background: 'transparent',
            border: 'none',
            color: copied ? '#34d399' : 'var(--text-muted)',
            cursor: 'pointer',
            display: 'flex',
            alignItems: 'center',
            gap: '0.35rem',
            fontSize: '0.75rem',
            fontFamily: 'inherit',
            transition: 'color 0.2s'
          }}
          title="Copy to clipboard"
        >
          {copied ? <Check size={13} /> : <Copy size={13} />}
          {copied ? 'Copied' : 'Copy'}
        </button>
      </div>
      <pre className="json-viewer" style={{ margin: 0, maxHeight: '280px', borderRadius: '0 0 8px 8px' }}>
        <code>{formatted}</code>
      </pre>
    </div>
  );
};
