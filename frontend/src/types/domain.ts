
export interface InstallRecord {
	id: number;
	aircraftPartId: number;
	aircraftModel: string;
	aircraftSerial: string;
	location: string;
	installedBy: string;
	installedAt: string;
	installRequestId: string;
	removedBy?: string;
	removedAt?: string | null;
	removeReason?: string;
	removeRequestId?: string;
}

export interface InstallBlocker {
	recordId: number;
	partId: number;
	partCode: string;
	partName: string;
	aircraftModel: string;
	aircraftSerial: string;
	location: string;
	installedBy: string;
}

export interface InstallInput {
	expectedVersion: number;
	aircraftModel: string;
	aircraftSerial: string;
	location: string;
	installedBy?: string;
}

export interface UninstallInput {
	expectedVersion: number;
	reason: string;
}

export interface DomainRecord {
  id: number;
  code: string;
  name: string;
  status: string;
  version: number;
  description: string;
  facility: string;
  owner: string;
  category: string;
  riskLevel: 'low' | 'medium' | 'high' | 'critical';
  metricValue: number;
  metricUnit: string;
  effectiveAt: string;
  evidence: string;
	relatedCode: string;
	preparedBy?: string;
	verifiedBy?: string;
	submittedBy?: string;
	reviewedBy?: string;
	reviewReason?: string;
	revisions?: VersionRevision[];
	installRecords?: InstallRecord[];
	createdAt: string;
	updatedAt: string;
}

export interface VersionRevision {
  id: number;
  version: number;
  status: string;
  evidence: string;
  actor: string;
  requestId: string;
  action: string;
  reason: string;
  createdAt: string;
}

export interface PageMeta { page: number; pageSize: number; total: number }
export interface ApiEnvelope<T> { data: T; error?: string; message?: string; meta?: PageMeta; details?: { reason?: string; blocker?: InstallBlocker } }
export interface UserSession { token: string; username: string; displayName: string; role: string; expiresIn: number }
export interface AuditLog {
  id: number; requestId: string; actor: string; action: string; entityType: string;
  entityId: number; beforeState: string; afterState: string; detail: string; createdAt: string;
}
export interface EntityConfig {
  key: string;
  path: string;
  label: string;
  statuses: readonly string[];
  primaryTransitions: Readonly<Record<string, string>>;
}
