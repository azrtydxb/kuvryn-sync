import React from 'react';
import { IconButton } from '../actions/IconButton.jsx';

export function Drawer({ open = true, title, description, onClose, children, footer, width, inline, className = '', style }) {
  React.useEffect(() => {
    if (!open || inline || !onClose) return;
    const k = (e) => { if (e.key === 'Escape') onClose(); };
    window.addEventListener('keydown', k);
    return () => window.removeEventListener('keydown', k);
  }, [open, inline, onClose]);
  if (!open) return null;
  const panel = (
    <aside className={'az-drawer' + (inline ? ' az-drawer--inline' : '') + (className ? ' ' + className : '')} role="dialog" aria-modal={inline ? undefined : 'true'} aria-label={title} style={{ width, ...style }} onClick={(e) => e.stopPropagation()}>
      <div className="az-drawer__head">
        <div><h2 className="az-drawer__title">{title}</h2>{description && <p className="az-drawer__desc">{description}</p>}</div>
        {onClose && <IconButton icon="x" label="Close" onClick={onClose} />}
      </div>
      <div className="az-drawer__body">{children}</div>
      {footer && <div className="az-drawer__foot">{footer}</div>}
    </aside>
  );
  if (inline) return panel;
  return <div className="az-drawer-backdrop" onClick={onClose}>{panel}</div>;
}

export function PropertyList({ items = [], className = '', style }) {
  return (
    <dl className={('az-props ' + className).trim()} style={style}>
      {items.map((it, i) => (
        <React.Fragment key={i}><dt>{it.label}</dt><dd className={it.mono ? 'az-props__mono' : undefined}>{it.value}</dd></React.Fragment>
      ))}
    </dl>
  );
}
