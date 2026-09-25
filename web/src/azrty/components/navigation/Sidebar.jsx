import { Logo } from '../brand/Logo.jsx';
import { Icon } from '../brand/Icon.jsx';
import { IconButton } from '../actions/IconButton.jsx';
import { Select } from '../forms/Select.jsx';
import { Avatar } from '../identity/Avatar.jsx';

export function Sidebar({ items = [], active, onSelect, workspace, workspaces = [], onWorkspaceChange, workspaceLabel = 'Tenant', status = 'Connected to control plane', user, onSignOut, className = '', style }) {
  return (
    <aside className={('az-sidebar ' + className).trim()} style={style}>
      <div className="az-sidebar__brand"><Logo size={30} /></div>
      {workspaces.length > 0 && (
        <div className="az-sidebar__workspace">
          <span className="az-eyebrow">{workspaceLabel}</span>
          <Select size="sm" value={workspace} options={workspaces} onChange={(e) => onWorkspaceChange && onWorkspaceChange(e.target.value)} />
        </div>
      )}
      <nav className="az-nav">
        {items.map((it) => (
          <button key={it.id} className={'az-nav__item' + (it.id === active ? ' az-nav__item--active' : '')} aria-current={it.id === active ? 'page' : undefined} onClick={() => onSelect && onSelect(it.id)}>
            <Icon name={it.icon} size={17} />
            <span>{it.label}</span>
            {it.badge != null && <span className="az-nav__badge">{it.badge}</span>}
          </button>
        ))}
      </nav>
      <div className="az-sidebar__foot">
        {status && <div className="az-connected"><span className="az-dot az-dot--pulse" />{status}</div>}
        {user && (
          <div className="az-profile">
            <Avatar name={user.name} size={32} />
            <span className="az-profile__meta"><b>{user.name}</b><small>{user.role}</small></span>
            <IconButton icon="log-out" label="Sign out" size={15} onClick={onSignOut} />
          </div>
        )}
      </div>
    </aside>
  );
}
