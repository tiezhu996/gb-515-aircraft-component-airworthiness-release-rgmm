import { useEffect, useMemo, useState } from 'react';
import { MetricCard } from '../components/common/MetricCard';
import { PartStatusBadge } from '../components/common/PartStatusBadge';
import { ConfirmDialog } from '../components/common/ConfirmDialog';
import { UiButton } from '../components/common/UiButton';
import { InstallDialog, type InstallFormValues } from '../components/common/InstallDialog';
import { UninstallDialog } from '../components/common/UninstallDialog';
import { InstallHistory } from '../components/common/InstallHistory';
import { useAuth } from '../hooks/useAuth';
import { useAircraftPartStore } from '../stores/aircraft-part';
import { ApiError } from '../api/client';
import { nextStatus, formatDate } from '../utils/format';
import type { DomainRecord, InstallRecord } from '../types/domain';

const primaryTransitions: Readonly<Record<string, string>> = {
	received: 'inspection', inspection: 'hold', hold: 'released', retired: 'released',
};

function activeInstallOf(item: DomainRecord) {
	return item.installRecords?.find((record) => record.removedAt == null);
}

function blockerText(error: unknown): string {
	if (error instanceof ApiError && error.blocker) {
		const b = error.blocker;
		if (error.code === 'install_part_active') {
			return `被已有在装记录挡住：本部件（${b.partCode}）由 ${b.installedBy} 装于 ${b.aircraftModel} 架次 ${b.aircraftSerial} 的 ${b.location}（履历 #${b.recordId}），尚未卸载。`;
		}
		if (error.code === 'install_slot_occupied') {
			return `被占用记录挡住：架次 ${b.aircraftSerial} 的安装位置 ${b.location} 已挂部件 ${b.partCode}（${b.partName}，装机人 ${b.installedBy}，履历 #${b.recordId}）。`;
		}
	}
	return error instanceof Error ? error.message : String(error);
}

export default function AircraftPartPage() {
	const { items, meta, loading, error, load, createRecord, transition, install, uninstall } = useAircraftPartStore();
	const { session } = useAuth();
	const [search, setSearch] = useState('');
	const [showCreate, setShowCreate] = useState(false);
	const [pending, setPending] = useState<{ item: DomainRecord; status: string } | null>(null);
	const [installTarget, setInstallTarget] = useState<DomainRecord | null>(null);
	const [uninstallTarget, setUninstallTarget] = useState<DomainRecord | null>(null);
	const [expanded, setExpanded] = useState<Set<number>>(new Set());
	const [dialogError, setDialogError] = useState('');
	const [busy, setBusy] = useState(false);

	useEffect(() => { void load('parts'); }, [load]);
	const highRisk = useMemo(() => items.filter((item) => ['high', 'critical'].includes(item.riskLevel)).length, [items]);
	const installedCount = useMemo(() => items.filter((item) => item.status === 'installed').length, [items]);

	const role = session?.role || 'viewer';
	const canOperate = role === 'operator' || role === 'reviewer' || role === 'admin';

	const createDemo = async () => {
		await createRecord('parts', { code: `AP-${Date.now().toString().slice(-6)}`, name: '新增航空部件',
			description: '通过前端工作台创建的部件记录', facility: '默认作业区', owner: '现场操作员', category: '常规', riskLevel: 'medium',
			metricValue: 25, metricUnit: 'unit', effectiveAt: new Date().toISOString(), evidence: '已完成创建前检查', relatedCode: '' });
		setShowCreate(false);
	};

	const toggleExpand = (id: number) => setExpanded((prev) => {
		const nextSet = new Set(prev);
		if (nextSet.has(id)) nextSet.delete(id); else nextSet.add(id);
		return nextSet;
	});

	const confirmInstall = async (values: InstallFormValues) => {
		if (!installTarget) return;
		setBusy(true);
		setDialogError('');
		try {
			await install(installTarget, {
				expectedVersion: installTarget.version,
				aircraftModel: values.aircraftModel.trim(),
				aircraftSerial: values.aircraftSerial.trim(),
				location: values.location.trim(),
				installedBy: values.installedBy.trim(),
			});
			setInstallTarget(null);
		} catch (reason) {
			setDialogError(blockerText(reason));
		} finally {
			setBusy(false);
		}
	};

	const confirmUninstall = async (reason: string) => {
		if (!uninstallTarget) return;
		setBusy(true);
		setDialogError('');
		try {
			await uninstall(uninstallTarget, { expectedVersion: uninstallTarget.version, reason });
			setUninstallTarget(null);
		} catch (reason) {
			setDialogError(blockerText(reason));
		} finally {
			setBusy(false);
		}
	};

	return <main className="workspace">
		<header className="page-header"><div><p className="eyebrow">业务工作台</p><h1>航空部件</h1><p>统一管理部件状态、风险、证据与装机履历；放行出库后登记装机，卸载回检查。</p></div>{canOperate ? <UiButton onClick={() => setShowCreate(true)}>新增航空部件</UiButton> : <span className="access-note">只读权限</span>}</header>
		<section className="metrics">
			<MetricCard label="记录总数" value={meta.total} detail="当前筛选范围" />
			<MetricCard label="已安装" value={installedCount} detail="在装部件数量" />
			<MetricCard label="高风险" value={highRisk} detail="需要优先复核" />
			<MetricCard label="状态种类" value={new Set(items.map((item) => item.status)).size} detail="状态机覆盖" />
		</section>
		<section className="toolbar">
			<input aria-label="搜索" placeholder="搜索部件编码或名称" value={search} onChange={(event) => setSearch(event.target.value)} />
			<UiButton onClick={() => void load('parts', search)}>查询</UiButton>
			<button className="link-button" onClick={() => { setSearch(''); void load('parts'); }}>重置</button>
		</section>
		{error && <div className="alert" role="alert">{error}</div>}
		<section className="table-shell" aria-busy={loading}><table><thead><tr>
			<th>编码</th><th>名称</th><th>状态</th><th>风险</th><th>责任人</th><th>当前装机</th><th>更新时间</th><th>操作</th>
		</tr></thead><tbody>
			{items.map((item) => {
				const next = nextStatus(item.status, primaryTransitions);
				const active = activeInstallOf(item);
				const open = expanded.has(item.id);
				return <PartRow key={item.id} item={item} next={next} active={active} open={open} canOperate={canOperate}
					onToggle={() => toggleExpand(item.id)}
					onTransition={() => next && setPending({ item, status: next })}
					onInstall={() => { setDialogError(''); setInstallTarget(item); }}
					onUninstall={() => { setDialogError(''); setUninstallTarget(item); }}
					onRetire={() => setPending({ item, status: 'retired' })} />;
			})}
			{!items.length && !loading && <tr><td colSpan={8} className="empty">暂无记录</td></tr>}
		</tbody></table>{loading && <div className="loading">正在同步业务数据…</div>}</section>

		<ConfirmDialog open={showCreate} title="新增航空部件" onCancel={() => setShowCreate(false)} onConfirm={() => void createDemo().catch(() => undefined)}><p>将创建一条包含完整责任人、风险和证据信息的演示记录。</p></ConfirmDialog>
		<ConfirmDialog open={Boolean(pending)} title="确认状态迁移" onCancel={() => setPending(null)} onConfirm={() => { if (pending) void transition('parts', pending.item, pending.status).then(() => setPending(null)).catch(() => undefined); }}><p>状态迁移会写入不可覆盖的审计日志。</p><strong>{pending?.item.status} → {pending?.status}</strong></ConfirmDialog>
		{installTarget && <InstallDialog part={installTarget} defaultInstaller={session?.username || ''} busy={busy} error={error} blockerError={dialogError}
			onCancel={() => { setInstallTarget(null); setDialogError(''); }} onConfirm={(values) => void confirmInstall(values)} />}
		{uninstallTarget && <UninstallDialog part={uninstallTarget} active={activeInstallOf(uninstallTarget)} busy={busy} error={dialogError}
			onCancel={() => { setUninstallTarget(null); setDialogError(''); }} onConfirm={(reason) => void confirmUninstall(reason)} />}
	</main>;
}

function PartRow({
	item, next, active, open, canOperate, onToggle, onTransition, onInstall, onUninstall, onRetire,
}: {
	item: DomainRecord;
	next: string | null;
	active?: InstallRecord;
	open: boolean;
	canOperate: boolean;
	onToggle: () => void;
	onTransition: () => void;
	onInstall: () => void;
	onUninstall: () => void;
	onRetire: () => void;
}) {
	return <>
		<tr>
			<td><strong>{item.code}</strong></td>
			<td>{item.name}<small>{item.facility}</small></td>
			<td><PartStatusBadge status={item.status} /></td>
			<td>{item.riskLevel}</td>
			<td>{item.owner}</td>
			<td>{active
				? <span>{active.aircraftModel} · {active.aircraftSerial}<small>{active.location} · {active.installedBy}</small></span>
				: <span className="muted">-</span>}</td>
			<td>{formatDate(item.updatedAt)}</td>
			<td className="part-actions">
				{item.status === 'released' && canOperate && <>
					<button className="table-action" onClick={onInstall}>装机</button>
					<button className="table-action" onClick={onRetire}>退役</button>
				</>}
				{item.status === 'installed' && canOperate && <button className="table-action table-action--danger" onClick={onUninstall}>卸载</button>}
				{next && canOperate && item.status !== 'installed' && item.status !== 'released' && <button className="table-action" onClick={onTransition}>推进至 {next}</button>}
				{item.status === 'installed' && !canOperate && <span className="muted">只读</span>}
				<button className="table-action" onClick={onToggle} aria-expanded={open}>{open ? '收起履历' : `装机履历${item.installRecords?.length ? `（${item.installRecords.length}）` : ''}`}</button>
			</td>
		</tr>
		{open && <tr className="install-history-row"><td colSpan={8}><InstallHistory records={item.installRecords || []} /></td></tr>}
	</>;
}
