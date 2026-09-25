import { Icon } from '../brand/Icon.jsx';

export function Badge({ tone = 'neutral', dot, icon, children, className = '', style }) {
  return (
    <span className={('az-badge az-badge--' + tone + ' ' + className).trim()} style={style}>
      {dot && <span className="az-badge__dot" />}
      {icon && <Icon name={icon} size={11} />}
      {children}
    </span>
  );
}
