import { Icon } from '../brand/Icon.jsx';

export function Select({ label, hint, error, options = [], size, id, className = '', style, ...rest }) {
  const iid = id || (label ? 'az-' + String(label).toLowerCase().replace(/[^a-z0-9]+/g, '-') : undefined);
  const opts = options.map((o) => (typeof o === 'string' ? { value: o, label: o } : o));
  return (
    <div className={'az-field' + (error ? ' az-field--error' : '') + (className ? ' ' + className : '')} style={style}>
      {label && <label className="az-field__label" htmlFor={iid}>{label}</label>}
      <span className="az-input-wrap az-select-wrap">
        <select id={iid} className={'az-input az-select' + (size === 'sm' ? ' az-input--sm' : '')} {...rest}>
          {opts.map((o) => <option key={o.value} value={o.value}>{o.label}</option>)}
        </select>
        <Icon name="chevron-down" size={15} className="az-select__chev" />
      </span>
      {(error || hint) && <span className="az-field__hint">{error || hint}</span>}
    </div>
  );
}
