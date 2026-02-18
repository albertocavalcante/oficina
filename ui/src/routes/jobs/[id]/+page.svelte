<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { getJob, getLogs, streamLogs, cancelJob, type Job, type LogLine } from '$lib/api';

	let job = $state<Job | null>(null);
	let logs = $state<LogLine[]>([]);
	let logEl = $state<HTMLElement>();
	let autoScroll = $state(true);
	let cleanup: (() => void) | null = null;

	function scrollToBottom() {
		if (logEl && autoScroll) {
			logEl.scrollTop = logEl.scrollHeight;
		}
	}

	onMount(() => {
		const id = page.params.id;

		async function load() {
			job = await getJob(id);
			logs = await getLogs(id);
			requestAnimationFrame(scrollToBottom);

			if (job.status === 'pending' || job.status === 'running') {
				cleanup = streamLogs(
					id,
					(line) => {
						logs = [...logs, line];
						requestAnimationFrame(scrollToBottom);
					},
					async () => {
						job = await getJob(id);
					}
				);
			}
		}

		load();

		const interval = setInterval(async () => {
			job = await getJob(id);
		}, 5000);

		return () => {
			clearInterval(interval);
			if (cleanup) cleanup();
		};
	});

	async function cancel() {
		if (!job) return;
		await cancelJob(job.id);
		job = await getJob(job.id);
	}

	function statusColor(status: string): string {
		switch (status) {
			case 'succeeded': return 'var(--green)';
			case 'failed': return 'var(--red)';
			case 'running': return 'var(--accent)';
			case 'cancelled': return 'var(--text-dim)';
			default: return 'var(--yellow)';
		}
	}

	function formatTime(dateStr?: string): string {
		if (!dateStr) return '—';
		return new Date(dateStr).toLocaleString();
	}

	function duration(start?: string, end?: string): string {
		if (!start) return '—';
		const s = new Date(start).getTime();
		const e = end ? new Date(end).getTime() : Date.now();
		const sec = Math.floor((e - s) / 1000);
		if (sec < 60) return `${sec}s`;
		if (sec < 3600) return `${Math.floor(sec / 60)}m ${sec % 60}s`;
		return `${Math.floor(sec / 3600)}h ${Math.floor((sec % 3600) / 60)}m`;
	}

	function handleLogScroll() {
		if (!logEl) return;
		const atBottom = logEl.scrollHeight - logEl.scrollTop - logEl.clientHeight < 40;
		autoScroll = atBottom;
	}
</script>

{#if !job}
	<p class="loading">Loading...</p>
{:else}
	<div class="header">
		<a href="/" class="back">&larr; Jobs</a>
		<h1>{job.name}</h1>
		<span class="status" style="color: {statusColor(job.status)}">{job.status}</span>
	</div>

	<div class="details">
		<div class="detail-row">
			<span class="label">ID</span>
			<code>{job.id}</code>
		</div>
		<div class="detail-row">
			<span class="label">Command</span>
			<code>{job.command}</code>
		</div>
		{#if job.shell}
			<div class="detail-row">
				<span class="label">Shell</span>
				<code>{job.shell}</code>
			</div>
		{/if}
		{#if job.agentId}
			<div class="detail-row">
				<span class="label">Agent</span>
				<code>{job.agentId}</code>
			</div>
		{/if}
		<div class="detail-row">
			<span class="label">Created</span>
			<span>{formatTime(job.createdAt)}</span>
		</div>
		{#if job.startedAt}
			<div class="detail-row">
				<span class="label">Started</span>
				<span>{formatTime(job.startedAt)}</span>
			</div>
		{/if}
		{#if job.endedAt}
			<div class="detail-row">
				<span class="label">Ended</span>
				<span>{formatTime(job.endedAt)}</span>
			</div>
		{/if}
		<div class="detail-row">
			<span class="label">Duration</span>
			<span>{duration(job.startedAt, job.endedAt)}</span>
		</div>
		{#if job.exitCode !== undefined && job.exitCode !== null}
			<div class="detail-row">
				<span class="label">Exit code</span>
				<code class:exit-fail={job.exitCode !== 0}>{job.exitCode}</code>
			</div>
		{/if}
		{#if job.error}
			<div class="detail-row">
				<span class="label">Error</span>
				<span class="error-text">{job.error}</span>
			</div>
		{/if}
		{#if job.status === 'pending'}
			<button class="cancel-btn" onclick={cancel}>Cancel Job</button>
		{/if}
	</div>

	<h2>Logs</h2>
	<div class="log-container" bind:this={logEl} onscroll={handleLogScroll}>
		{#if logs.length === 0}
			<p class="log-empty">No output yet.</p>
		{:else}
			{#each logs as line}
				<div class="log-line" class:stderr={line.stream === 'stderr'}>{line.text}</div>
			{/each}
		{/if}
	</div>
{/if}

<style>
	.loading {
		color: var(--text-dim);
		text-align: center;
		padding: 48px 0;
	}
	.header {
		display: flex;
		align-items: center;
		gap: 12px;
		margin-bottom: 16px;
	}
	.back {
		font-size: 14px;
	}
	h1 {
		font-size: 22px;
		font-weight: 600;
		flex: 1;
	}
	.status {
		font-size: 13px;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.5px;
	}
	.details {
		background: var(--bg-card);
		border: 1px solid var(--border);
		border-radius: 8px;
		padding: 16px;
		margin-bottom: 24px;
	}
	.detail-row {
		display: flex;
		gap: 12px;
		padding: 4px 0;
		font-size: 14px;
	}
	.label {
		color: var(--text-dim);
		min-width: 80px;
		font-weight: 500;
	}
	code {
		font-family: var(--font-mono);
		font-size: 13px;
	}
	.exit-fail {
		color: var(--red);
	}
	.error-text {
		color: var(--red);
	}
	.cancel-btn {
		margin-top: 12px;
		background: none;
		border: 1px solid var(--red);
		color: var(--red);
		padding: 6px 16px;
		border-radius: 6px;
		cursor: pointer;
		font-size: 13px;
	}
	h2 {
		font-size: 18px;
		font-weight: 600;
		margin-bottom: 8px;
	}
	.log-container {
		background: #010409;
		border: 1px solid var(--border);
		border-radius: 8px;
		padding: 12px;
		max-height: 600px;
		overflow-y: auto;
		font-family: var(--font-mono);
		font-size: 13px;
		line-height: 1.6;
	}
	.log-empty {
		color: var(--text-dim);
	}
	.log-line {
		white-space: pre-wrap;
		word-break: break-all;
	}
	.log-line.stderr {
		color: var(--red);
	}
</style>
