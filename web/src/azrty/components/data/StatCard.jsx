import { Icon } from '../brand/Icon.jsx';
import { Sparkline } from './Sparkline.jsx';

export function StatCard({ label, value, unit, icon, sub, delta, trend, trendColor, className = '', style }) {
  return (
    <div className={('az-card az-stat ' + className).trim()} style={style}>
      <div className="az-stat__head"><span>{label}</span>{icon && <Icon name={icon} size={15} />}</div>
      <div className="az-stat__value">{value}{unit && <span className="az-stat__unit">{unit}</span>}</div>
      {(sub || delta) && (
        <div className="az-stat__sub">
          {delta && <span className={'az-stat__delta--' + (delta.direction || 'up')}>{delta.value}</span>}
          {sub && <span>{sub}</span>}
        </div>
      )}
      {trend && <div className="az-stat__spark"><Sparkline data={trend} color={trendColor} height={36} /></div>}
    </div>
  );
}
