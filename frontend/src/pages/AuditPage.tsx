
import { useEffect, useState } from 'react';
import { listAudits } from '../api/audit';
import type { AuditLog } from '../types/domain';
import { formatDate } from '../utils/format';

const actionLabels: Record<string, string> = {
	create: '创建', update: '更新', transition: '状态推进', delete: '删除',
	install: '装机', uninstall: '卸载',
};

export default function AuditPage() {
  const [logs, setLogs] = useState<AuditLog[]>([]); const [error, setError] = useState('');
  useEffect(() => { listAudits().then((result) => setLogs(result.data)).catch((reason) => setError(String(reason))); }, []);
  return <main className="workspace"><header className="page-header"><div><p className="eyebrow">治理与追踪</p><h1>操作审计</h1><p>记录操作者、请求 ID、实体和不可逆状态变化（含装机与卸载）。</p></div></header>{error && <div className="alert">{error}</div>}<section className="audit-list">{logs.map((log) => <article key={log.id}><time>{formatDate(log.createdAt)}</time><strong>{log.actor} · {actionLabels[log.action] || log.action}</strong><span>{log.entityType} #{log.entityId}</span><code>{log.beforeState || '-'} → {log.afterState || '-'}</code><small title={log.detail}>{log.requestId}</small></article>)}</section></main>;
}
