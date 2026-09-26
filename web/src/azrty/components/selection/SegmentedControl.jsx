import React from 'react';
import { Icon } from '../brand/Icon.jsx';

export function SegmentedControl({ options = [], value, defaultValue, onChange, mono, block, className = '', style, 'aria-label': ariaLabel }) {
  const opts = options.map((o) => (typeof o === 'string' ? { value: o, label: o } : o));
  const [inner, setInner] = React.useState(defaultValue != null ? defaultValue : opts[0] && opts[0].value);
  const cur = value != null ? value : inner;
  return (
    <div className={'az-seg' + (mono ? ' az-seg--mono' : '') + (block ? ' az-seg--block' : '') + (className ? ' ' + className : '')} role="radiogroup" aria-label={ariaLabel} style={style}>
      {opts.map((o) => (
        <button key={o.value} role="radio" aria-checked={o.value === cur} className={'az-seg__item' + (o.value === cur ? ' az-seg__item--active' : '')} onClick={() => { setInner(o.value); onChange && onChange(o.value); }}>
          {o.icon && <Icon name={o.icon} size={14} />}{o.label}
        </button>
      ))}
    </div>
  );
}
