import { Icon } from '../brand/Icon.jsx';

export function Button({ variant = 'primary', size = 'md', icon, iconRight, block, children, className = '', type = 'button', ...rest }) {
  const cls = ['az-btn', 'az-btn--' + variant, size !== 'md' && 'az-btn--' + size, block && 'az-btn--block', !children && 'az-btn--icon-only', className].filter(Boolean).join(' ');
  const is = size === 'sm' ? 13 : size === 'lg' ? 17 : 15;
  return (
    <button type={type} className={cls} {...rest}>
      {icon && <Icon name={icon} size={is} />}
      {children}
      {iconRight && <Icon name={iconRight} size={is} />}
    </button>
  );
}
