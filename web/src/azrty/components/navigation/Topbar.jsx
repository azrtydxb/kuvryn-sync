import React from 'react';
import { Icon } from '../brand/Icon.jsx';

export function Topbar({ crumbs = [], live = true, children, className = '', style }) {
  return (
    <header className={('az-topbar ' + className).trim()} style={style}>
      <nav className="az-crumbs" aria-label="Breadcrumb">
        {crumbs.map((c, i) => (
          <React.Fragment key={i}>
            {i > 0 && <Icon name="chevron-right" size={12} />}
            {i === crumbs.length - 1 ? <b>{c}</b> : <span>{c}</span>}
          </React.Fragment>
        ))}
      </nav>
      <div className="az-topbar__right">
        {live && <span className="az-live"><span className="az-dot az-dot--pulse" />LIVE</span>}
        {children}
      </div>
    </header>
  );
}
