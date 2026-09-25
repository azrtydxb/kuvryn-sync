export function Icon({ name, size = 16, color, className = '', style, label }) {
  return (
    <i
      className={('icon-' + name + ' az-icon ' + className).trim()}
      style={{ fontSize: size, color, ...style }}
      aria-hidden={label ? undefined : 'true'}
      aria-label={label}
      role={label ? 'img' : undefined}
    />
  );
}
