import React from 'react';
export function Tabs({ items = [], value, defaultValue, onChange, className = '', style }) {
  const [inner, setInner] = React.useState(defaultValue != null ? defaultValue : items[0] && items[0].id);
  const active = value != null ? value : inner;
  return (
    <div className={('az-tabs ' + className).trim()} role="tablist" style={style}>
      {items.map((it) => (
        <button key={it.id} role="tab" aria-selected={it.id === active} className={'az-tab' + (it.id === active ? ' az-tab--active' : '')} onClick={() => { setInner(it.id); onChange && onChange(it.id); }}>
          {it.label}
          {it.count != null && <span className="az-tab__count">{it.count}</span>}
        </button>
      ))}
    </div>
  );
}
