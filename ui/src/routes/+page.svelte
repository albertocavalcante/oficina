<script lang="ts">
	import { onMount } from 'svelte';
	import { listJobs, createJob, cancelJob, type Job } from '$lib/api';

	let jobs = $state<Job[]>([]);
	let name = $state('');
	let command = $state('');
	let shell = $state('');
	let submitting = $state(false);

	async function refresh() {
		jobs = (await listJobs()).sort(
			(a, b) => new Date(b.createdAt).getTime() - new Date(a.createdAt).getTime()
		);
	}

	onMount(() => {
		refresh();
		const interval = setInterval(refresh, 3000);
		return () => clearInterval(interval);
	});

	async function submit() {
		if (!command.trim()) return;
		submitting = true;
		await createJob(name || 'job', command, shell || undefined);
		command = '';
		name = '';
		await refresh();
		submitting = false;
	}

	async function cancel(id: string) {
		await cancelJob(id);
		await refresh();
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

	function ago(dateStr: string): string {
		const s = Math.floor((Date.now() - new Date(dateStr).getTime()) / 1000);
		if (s < 60) return `${s}s ago`;
		if (s < 3600) return `${Math.floor(s / 60)}m ago`;
		return `${Math.floor(s / 3600)}h ago`;
	}
</script>

<div class="header">
	<h1>Jobs</h1>
</div>

<form class="submit-form" onsubmit={(e) => { e.preventDefault(); submit(); }}>
	<input bind:value={name} placeholder="Job name" class="input name-input" />
	<input bind:value={command} placeholder="Command to run..." class="input cmd-input" />
	<input bind:value={shell} placeholder="Shell" class="input shell-input" />
	<button type="submit" class="btn" disabled={submitting || !command.trim()}>
		{submitting ? 'Submitting...' : 'Submit'}
	</button>
</form>

{#if jobs.length === 0}
	<p class="empty">No jobs yet. Submit one above.</p>
{:else}
	<div class="job-list">
		{#each jobs as job (job.id)}
			<div class="job-card">
				<div class="job-header">
					<a href="/jobs/{job.id}" class="job-name">{job.name}</a>
					<span class="status" style="color: {statusColor(job.status)}">{job.status}</span>
				</div>
				<code class="job-cmd">{job.command}</code>
				<div class="job-meta">
					<span>{job.id.slice(0, 8)}</span>
					<span>{ago(job.createdAt)}</span>
					{#if job.agentId}
						<span>agent: {job.agentId.slice(0, 8)}</span>
					{/if}
					{#if job.exitCode !== undefined && job.exitCode !== null}
						<span>exit: {job.exitCode}</span>
					{/if}
					{#if job.status === 'pending'}
						<button class="cancel-btn" onclick={() => cancel(job.id)}>Cancel</button>
					{/if}
				</div>
			</div>
		{/each}
	</div>
{/if}

<style>
	.header {
		margin-bottom: 16px;
	}
	h1 {
		font-size: 24px;
		font-weight: 600;
	}
	.submit-form {
		display: flex;
		gap: 8px;
		margin-bottom: 24px;
	}
	.input {
		background: var(--bg-input);
		border: 1px solid var(--border);
		color: var(--text);
		padding: 8px 12px;
		border-radius: 6px;
		font-size: 14px;
		font-family: var(--font-mono);
	}
	.name-input { width: 140px; }
	.cmd-input { flex: 1; }
	.shell-input { width: 80px; }
	.btn {
		background: var(--accent);
		color: #000;
		border: none;
		padding: 8px 16px;
		border-radius: 6px;
		font-weight: 600;
		cursor: pointer;
	}
	.btn:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}
	.empty {
		color: var(--text-dim);
		text-align: center;
		padding: 48px 0;
	}
	.job-list {
		display: flex;
		flex-direction: column;
		gap: 8px;
	}
	.job-card {
		background: var(--bg-card);
		border: 1px solid var(--border);
		border-radius: 8px;
		padding: 12px 16px;
	}
	.job-header {
		display: flex;
		justify-content: space-between;
		align-items: center;
		margin-bottom: 4px;
	}
	.job-name {
		font-weight: 600;
		font-size: 15px;
	}
	.status {
		font-size: 13px;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.5px;
	}
	.job-cmd {
		display: block;
		font-size: 13px;
		color: var(--text-dim);
		font-family: var(--font-mono);
		margin-bottom: 8px;
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}
	.job-meta {
		display: flex;
		gap: 12px;
		font-size: 12px;
		color: var(--text-dim);
	}
	.cancel-btn {
		background: none;
		border: 1px solid var(--red);
		color: var(--red);
		padding: 1px 8px;
		border-radius: 4px;
		cursor: pointer;
		font-size: 12px;
	}
</style>
