import type { InstallRecord } from '../../types/domain';
import { formatDate } from '../../utils/format';
import { PartStatusBadge } from './PartStatusBadge';

// InstallHistory renders the append-only 装机履历 of one part. Closed records
// stay visible forever so 装到哪架机上 remains traceable after uninstall.
export function InstallHistory({ records }: { records: InstallRecord[] }) {
	if (!records.length) {
		return <div className="empty">暂无装机履历，部件放行后可登记装机。</div>;
	}
	const open = records.some((record) => record.removedAt == null);
	return <div className="install-history">
		<div className="install-history__head">
			<strong>装机履历（{records.length}）</strong>
			{open
				? <PartStatusBadge status="installed" />
				: <span className="muted">当前未装机，历史记录永久保留</span>}
		</div>
		<ol className="install-timeline">
			{records.map((record) => <li key={record.id} className={record.removedAt == null ? 'install-row install-row--active' : 'install-row'}>
				<div className="install-row__title">
					<strong>#{record.id} · {record.aircraftModel} / 架次 {record.aircraftSerial}</strong>
					<span className="install-row__slot">安装位置：{record.location}</span>
				</div>
				<div className="install-row__meta">
					<span>装机人：{record.installedBy}</span>
					<span>装机时间：{formatDate(record.installedAt)}</span>
					<code title={record.installRequestId}>请求 {record.installRequestId}</code>
					{record.removedAt == null
						? <span className="install-row__state">在装中</span>
						: <span className="install-row__state install-row__state--closed">
							已卸载 · {record.removedBy} · {formatDate(record.removedAt)}
						</span>}
				</div>
				{record.removedAt != null && <div className="install-row__reason">
					卸载原因：{record.removeReason || '-'}
					{record.removeRequestId && <code title={record.removeRequestId}> · 请求 {record.removeRequestId}</code>}
				</div>}
			</li>)}
		</ol>
	</div>;
}
