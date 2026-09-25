import { Icon } from '../brand/Icon.jsx';

const ALERT_ICONS = { info: 'info', good: 'circle-check', warn: 'triangle-alert', bad: 'circle-x' };

export function Alert({ tone = 'info', title, children, icon, action, className = '', style }) {
  return (
    <div className={('az-alert az-alert--' + tone + ' ' + className).trim()} role={tone === 'bad' ? 'alert' : 'status'} style={style}>
      <Icon name={icon || ALERT_ICONS[tone]} size={16} />
      <div className="az-alert__body">
        {title && <span className="az-alert__title">{title}</span>}
        {children && <span>{children}</span>}
      </div>
      {action && <div className="az-alert__action">{action}</div>}
    </div>
  );
}
