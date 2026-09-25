export function ProductLogo({ name, sub, tagline, pillar, emblem, emblemLight, layout = 'stacked', size = 160, className = '', style }) {
  const label = [name, sub].filter(Boolean).join(' ');
  const em = emblem
    ? <>
        <img className="az-plogo__em az-plogo__em--dark" src={emblem} alt="" />
        <img className="az-plogo__em az-plogo__em--light" src={emblemLight || emblem} alt="" />
      </>
    : <span className="az-plogo__ph">Emblem</span>;
  const s = layout === 'horizontal' ? Math.round(size) : size;
  return (
    <span className={'az-plogo az-plogo--' + layout + (className ? ' ' + className : '')} data-pillar={pillar} role="img" aria-label={label} style={{ '--az-plogo-size': s + 'px', ...style }}>
      {em}
      {layout !== 'icon' && (
        <span className="az-plogo__text">
          <span className="az-plogo__name">{name}</span>
          {sub && <span className="az-plogo__sub"><span className="az-plogo__subtext">{sub}</span></span>}
          {tagline && layout === 'stacked' && <span className="az-plogo__tag">{tagline}</span>}
        </span>
      )}
    </span>
  );
}
