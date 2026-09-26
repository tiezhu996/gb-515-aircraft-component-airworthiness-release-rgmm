import { useState } from 'react';
import type { DomainRecord } from '../../types/domain';
import { UiButton } from './UiButton';

export interface InstallFormValues {
	aircraftModel: string;
	aircraftSerial: string;
	location: string;
	installedBy: string;
}

// InstallDialog collects 机型、架次、安装位置、装机人 for one 装机登记.
export function InstallDialog({
	part, defaultInstaller, busy, error, blockerError,
	onCancel, onConfirm,
}: {
	part: DomainRecord;
	defaultInstaller: string;
	busy: boolean;
	error: string;
	blockerError: string;
	onCancel: () => void;
	onConfirm: (values: InstallFormValues) => void;
}) {
	const [values, setValues] = useState<InstallFormValues>({
		aircraftModel: '', aircraftSerial: '', location: '', installedBy: defaultInstaller,
	});
	const update = (key: keyof InstallFormValues) => (event: React.ChangeEvent<HTMLInputElement>) =>
		setValues((prev) => ({ ...prev, [key]: event.target.value }));
	const valid = values.aircraftModel.trim() && values.aircraftSerial.trim() && values.location.trim() && values.installedBy.trim();
	return <div className="modal-backdrop"><section className="modal" role="dialog" aria-modal="true">
		<h2>登记装机 · {part.code}</h2>
		<p className="muted">部件放行出库后登记装机履历，状态将由「已放行」推进为「已安装」。同一部件不可重复在装，同一架次的同一位置不可挂两件部件。</p>
		<form className="install-form" onSubmit={(event) => { event.preventDefault(); if (valid && !busy) onConfirm(values); }}>
			<label>机型
				<input value={values.aircraftModel} onChange={update('aircraftModel')} placeholder="例如 C919" maxLength={120} required />
			</label>
			<label>架次（机号）
				<input value={values.aircraftSerial} onChange={update('aircraftSerial')} placeholder="例如 B-001" maxLength={80} required />
			</label>
			<label>安装位置
				<input value={values.location} onChange={update('location')} placeholder="例如 左翼-1号挂点" maxLength={120} required />
			</label>
			<label>装机人
				<input value={values.installedBy} onChange={update('installedBy')} placeholder="执行装机的人员" maxLength={80} required />
			</label>
			{(error || blockerError) && <div className="alert" role="alert">
				{blockerError || error}
			</div>}
			<footer><button type="button" className="link-button" onClick={onCancel}>取消</button>
				<UiButton onClick={() => onConfirm(values)} disabled={!valid || busy}>{busy ? '提交中…' : '确认装机'}</UiButton></footer>
		</form>
	</section></div>;
}
