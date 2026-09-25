const AV_STATUS = { online: 'var(--az-good)', busy: 'var(--az-warn)', offline: 'var(--az-faint)' };

export function Avatar({ name = '', src, size = 32, tone = 'neutral', status, className = '', style }) {
  const initials = name.split(/\s+/).filter(Boolean).map((p) => p[0]).slice(0, 2).join('').toUpperCase();
  return (
    <span className={'az-av' + (tone !== 'neutral' ? ' az-av--' + tone : '') + (className ? ' ' + className : '')} style={{ width: size, height: size, fontSize: Math.round(size * 0.36), ...style }} title={name} aria-label={name} role="img">
      {src ? <img src={src} alt="" /> : initials}
      {status && <span className="az-av__status" style={{ background: AV_STATUS[status] }}></span>}
    </span>
  );
}

export function AvatarGroup({ people = [], max = 4, size = 28, className = '', style }) {
  const shown = people.slice(0, max);
  const rest = people.length - shown.length;
  return (
    <span className={('az-av-group ' + className).trim()} style={style}>
      {shown.map((p, i) => <Avatar key={i} size={size} {...(typeof p === 'string' ? { name: p } : p)} />)}
      {rest > 0 && <span className="az-av az-av--more" style={{ width: size, height: size, fontSize: Math.round(size * 0.34) }}>+{rest}</span>}
    </span>
  );
}
