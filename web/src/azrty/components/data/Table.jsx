export function Table({ columns = [], rows = [], onRowClick, rowKey = 'id', className = '', style }) {
  return (
    <div className={('az-table-wrap ' + className).trim()} style={style}>
      <table className="az-table">
        <thead>
          <tr>{columns.map((c) => <th key={c.key} style={{ textAlign: c.align || 'left', width: c.width }}>{c.label}</th>)}</tr>
        </thead>
        <tbody>
          {rows.map((r, i) => (
            <tr key={r[rowKey] != null ? r[rowKey] : i} className={onRowClick ? 'az-table__row--click' : undefined} onClick={onRowClick ? () => onRowClick(r) : undefined}>
              {columns.map((c, ci) => (
                <td key={c.key} style={{ textAlign: c.align || 'left' }} className={[ci === 0 && c.primary !== false ? 'az-table__primary' : '', c.mono ? 'az-table__mono' : ''].join(' ').trim() || undefined}>
                  {c.render ? c.render(r) : r[c.key]}
                </td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
