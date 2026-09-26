
import { create } from 'zustand';
import { request } from '../api/client';
import { installAircraftPart, uninstallAircraftPart } from '../api/aircraft-part';
import type { ApiEnvelope, DomainRecord, InstallInput, PageMeta, UninstallInput } from '../types/domain';

export interface AircraftPartState {
	items: DomainRecord[];
	meta: PageMeta;
	loading: boolean;
	error: string;
	load: (path: string, search?: string) => Promise<void>;
	createRecord: (path: string, input: Partial<DomainRecord>) => Promise<void>;
	transition: (path: string, item: DomainRecord, status: string) => Promise<void>;
	install: (item: DomainRecord, input: InstallInput) => Promise<void>;
	uninstall: (item: DomainRecord, input: UninstallInput) => Promise<void>;
}

async function listParts(search = ''): Promise<ApiEnvelope<DomainRecord[]>> {
	return request<DomainRecord[]>(`/parts?page=1&pageSize=20&search=${encodeURIComponent(search)}`);
}

export const useAircraftPartStore = create<AircraftPartState>((set, get) => ({
	items: [], meta: { page: 1, pageSize: 20, total: 0 }, loading: false, error: '',
	load: async (_path, search = '') => {
		set({ loading: true, error: '' });
		try {
			const result = await listParts(search);
			set({ items: result.data, meta: result.meta || { page: 1, pageSize: 20, total: result.data.length }, loading: false });
		} catch (error) { set({ error: error instanceof Error ? error.message : String(error), loading: false }); }
	},
	createRecord: async (path, input) => {
		set({ loading: true, error: '' });
		try {
			await request<DomainRecord>(`/${path}`, { method: 'POST', body: JSON.stringify(input) });
			await get().load(path);
		} catch (error) { set({ error: error instanceof Error ? error.message : String(error), loading: false }); throw error; }
	},
	transition: async (path, item, status) => {
		set({ loading: true, error: '' });
		try {
			await request<DomainRecord>(`/${path}/${item.id}/transition`, { method: 'POST', body: JSON.stringify({ status, expectedVersion: item.version, reason: '前端工作台人工确认' }) });
			await get().load(path);
		} catch (error) { set({ error: error instanceof Error ? error.message : String(error), loading: false }); throw error; }
	},
	install: async (item, input) => {
		set({ loading: true, error: '' });
		try {
			await installAircraftPart(item.id, input);
			await get().load('parts');
		} catch (error) { set({ error: error instanceof Error ? error.message : String(error), loading: false }); throw error; }
	},
	uninstall: async (item, input) => {
		set({ loading: true, error: '' });
		try {
			await uninstallAircraftPart(item.id, input);
			await get().load('parts');
		} catch (error) { set({ error: error instanceof Error ? error.message : String(error), loading: false }); throw error; }
	},
}));
