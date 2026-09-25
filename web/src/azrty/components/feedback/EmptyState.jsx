import { Icon } from '../brand/Icon.jsx';

export function EmptyState({ icon = 'inbox', title, description, action, className = '', style }) {
  return (
    <div className={('az-empty ' + className).trim()} style={style}>
      <Icon name={icon} size={28} />
      <h3 className="az-empty__title">{title}</h3>
      {description && <p className="az-empty__text">{description}</p>}
      {action && <div className="az-empty__action">{action}</div>}
    </div>
  );
}
