export function Tooltip({ content, side = 'top', kbd, open, children, className = '', style }) {
  return (
    <span className={'az-tip' + (side === 'bottom' ? ' az-tip--bottom' : '') + (open ? ' az-tip--open' : '') + (className ? ' ' + className : '')} style={style}>
      {children}
      <span className="az-tip__bubble" role="tooltip">{content}{kbd && <span className="az-tip__kbd">{kbd}</span>}</span>
    </span>
  );
}
