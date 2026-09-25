import { Icon } from '../brand/Icon.jsx';

export function IconButton({ icon, label, size = 16, className = '', type = 'button', ...rest }) {
  return (
    <button type={type} className={('az-iconbtn ' + className).trim()} aria-label={label} title={label} {...rest}>
      <Icon name={icon} size={size} />
    </button>
  );
}
