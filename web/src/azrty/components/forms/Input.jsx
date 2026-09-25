import { Icon } from '../brand/Icon.jsx';

export function Input({ label, hint, error, icon, mono, size, id, className = '', style, ...rest }) {
  const iid = id || (label ? 'az-' + String(label).toLowerCase().replace(/[^a-z0-9]+/g, '-') : undefined);
  return (
    <div className={'az-field' + (error ? ' az-field--error' : '') + (className ? ' ' + className : '')} style={style}>
      {label && <label className="az-field__label" htmlFor={iid}>{label}</label>}
      <span className={'az-input-wrap' + (icon ? ' az-input-wrap--icon' : '')}>
        {icon && <Icon name={icon} size={15} />}
        <input id={iid} className={'az-input' + (mono ? ' az-input--mono' : '') + (size === 'sm' ? ' az-input--sm' : '')} aria-invalid={error ? 'true' : undefined} {...rest} />
      </span>
      {(error || hint) && <span className="az-field__hint">{error || hint}</span>}
    </div>
  );
}
