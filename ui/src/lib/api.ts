const BASE = '';

export interface Job {
	id: string;
	name: string;
	command: string;
	shell?: string;
	status: 'pending' | 'running' | 'succeeded' | 'failed' | 'cancelled';
	agentId?: string;
	exitCode?: number;
	error?: string;
	createdAt: string;
	startedAt?: string;
	endedAt?: string;
}

export interface Agent {
	id: string;
	name: string;
	os: string;
	arch: string;
	labels?: string[];
	status: 'online' | 'offline' | 'busy';
	lastSeen: string;
	currentJob?: string;
}

export interface LogLine {
	ts: string;
	stream: string;
	text: string;
}

export async function listJobs(): Promise<Job[]> {
	const res = await fetch(`${BASE}/api/jobs`);
	return res.json();
}

export async function getJob(id: string): Promise<Job> {
	const res = await fetch(`${BASE}/api/jobs/${id}`);
	return res.json();
}

export async function createJob(name: string, command: string, shell?: string): Promise<Job> {
	const res = await fetch(`${BASE}/api/jobs`, {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ name, command, shell })
	});
	return res.json();
}

export async function cancelJob(id: string): Promise<void> {
	await fetch(`${BASE}/api/jobs/${id}/cancel`, { method: 'POST' });
}

export async function getLogs(id: string): Promise<LogLine[]> {
	const res = await fetch(`${BASE}/api/jobs/${id}/logs`);
	return res.json();
}

export async function listAgents(): Promise<Agent[]> {
	const res = await fetch(`${BASE}/api/agents`);
	return res.json();
}

export function streamLogs(jobId: string, onLine: (line: LogLine) => void, onDone: () => void): () => void {
	const es = new EventSource(`${BASE}/api/jobs/${jobId}/stream`);
	es.onmessage = (e) => {
		onLine(JSON.parse(e.data));
	};
	es.addEventListener('done', () => {
		onDone();
		es.close();
	});
	es.onerror = () => {
		es.close();
	};
	return () => es.close();
}
