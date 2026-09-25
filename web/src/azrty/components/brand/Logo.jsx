export function Logo({ variant = 'lockup', size = 32, className = '', style }) {
  const stroke = size <= 20 ? 6.5 : 5.5;
  const mark = (
    <svg width={size} height={size} viewBox="0 0 40 40" aria-hidden="true" style={{ display: 'block', flex: 'none' }}>
      <circle cx="20" cy="20" r="20" style={{ fill: 'var(--az-brand-sky)' }} />
      <polyline points="16,11 25.5,20 16,29" fill="none" strokeWidth={stroke} strokeLinecap="round" strokeLinejoin="round" style={{ stroke: 'var(--az-brand-mint)' }} />
    </svg>
  );
  if (variant === 'mark') return <span className={className} style={{ display: 'inline-flex', ...style }} role="img" aria-label="Azrty">{mark}</span>;
  return (
    <span className={('az-logo ' + className).trim()} style={{ gap: Math.round(size * 0.33), ...style }} role="img" aria-label="Azrty">
      {mark}
      <span className="az-logo__word" style={{ fontSize: Math.round(size * 0.82) }}>AZRTY</span>
    </span>
  );
}
