import React from 'react';
import { IconButton } from '../actions/IconButton.jsx';

export function CodeBlock({ code = '', title, copyable = true, maxHeight, className = '', style }) {
  const [copied, setCopied] = React.useState(false);
  const copy = () => {
    if (navigator.clipboard) navigator.clipboard.writeText(code).catch(() => {});
    setCopied(true);
    setTimeout(() => setCopied(false), 1400);
  };
  return (
    <div className={('az-code ' + className).trim()} style={style}>
      {(title || copyable) && (
        <div className="az-code__head">
          <span>{title}</span>
          {copyable && <IconButton icon={copied ? 'check' : 'copy'} label={copied ? 'Copied' : 'Copy'} size={14} onClick={copy} />}
        </div>
      )}
      <pre style={{ maxHeight }}>{code}</pre>
    </div>
  );
}
