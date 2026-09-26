import { useState } from 'react';
import type { DomainRecord, InstallRecord } from '../../types/domain';
import { UiButton } from './UiButton';

// UninstallDialog forces the operator to state a 卸载原因. The part returns to
// 检查中 and the open 装机履历 record is closed, not deleted.
export function UninstallDialog({
	part, active, busy, error, onCancel, onConfirm,
}: {
	part: DomainRecord;
	active?: InstallRecord;
	busy: boolean;
	error: string;
	onCancel: () => void;
	onConfirm: (reason: string) => void;
}) {
	const [reason, setReason] = useState('');
	const valid = reason.trim().length > 0;
	return <div className="modal-backdrop"><section className="modal" role="dialog" aria-modal="true">
		<h2>登记卸载 · {part.code}</h2>
		{active && <p className="muted">
			当前在装：{active.aircraftModel} / 架次 {active.aircraftSerial} · {active.location}（装机人 {active.installedBy}）
		</p>}
		<p className="muted">卸载后部件回到「检查中」，装机履历保留在部件详情中可随时查阅。</p>
		<form className="install-form" onSubmit={(event) => { event.preventDefault(); if (valid && !busy) onConfirm(reason.trim()); }}>
			<label>卸载原因（必填）
				<textarea value={reason} onChange={(event) => setReason(event.target.value)}
					placeholder="例如 定检到期拆下、故障排查拆下、到寿更换" maxLength={1000} rows={4} required />
			</label>
			{error && <div className="alert" role="alert">{error}</div>}
			<footer><button type="button" className="link-button" onClick={onCancel}>取消</button>
				<UiButton danger onClick={() => onConfirm(reason.trim())} disabled={!valid || busy}>{busy ? '提交中…' : '确认卸载'}</UiButton></footer>
		</form>
	</section></div>;
}
