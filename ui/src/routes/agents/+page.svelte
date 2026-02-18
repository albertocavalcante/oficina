<script lang="ts">
	import { onMount } from 'svelte';
	import { listAgents, type Agent } from '$lib/api';

	let agents = $state<Agent[]>([]);

	async function refresh() {
		agents = await listAgents();
	}

	onMount(() => {
		refresh();
		const interval = setInterval(refresh, 5000);
		return () => clearInterval(interval);
	});

	function statusColor(status: string): string {
		switch (status) {
			case 'online': return 'var(--green)';
			case 'busy': return 'var(--accent)';
			default: return 'var(--text-dim)';
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
	<h1>Agents</h1>
</div>

{#if agents.length === 0}
	<p class="empty">No agents registered.</p>
{:else}
	<div class="agent-list">
		{#each agents as agent (agent.id)}
			<div class="agent-card">
				<div class="agent-header">
					<span class="agent-name">{agent.name}</span>
					<span class="status" style="color: {statusColor(agent.status)}">{agent.status}</span>
				</div>
				<div class="agent-meta">
					<span>{agent.id.slice(0, 8)}</span>
					<span>{agent.os}/{agent.arch}</span>
					<span>seen {ago(agent.lastSeen)}</span>
					{#if agent.labels && agent.labels.length > 0}
						<span class="labels">{agent.labels.join(', ')}</span>
					{/if}
				</div>
				{#if agent.currentJob}
					<div class="current-job">
						Running: <a href="/jobs/{agent.currentJob}">{agent.currentJob.slice(0, 8)}</a>
					</div>
				{/if}
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
	.empty {
		color: var(--text-dim);
		text-align: center;
		padding: 48px 0;
	}
	.agent-list {
		display: flex;
		flex-direction: column;
		gap: 8px;
	}
	.agent-card {
		background: var(--bg-card);
		border: 1px solid var(--border);
		border-radius: 8px;
		padding: 12px 16px;
	}
	.agent-header {
		display: flex;
		justify-content: space-between;
		align-items: center;
		margin-bottom: 4px;
	}
	.agent-name {
		font-weight: 600;
		font-size: 15px;
	}
	.status {
		font-size: 13px;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.5px;
	}
	.agent-meta {
		display: flex;
		gap: 12px;
		font-size: 12px;
		color: var(--text-dim);
	}
	.labels {
		color: var(--accent);
	}
	.current-job {
		margin-top: 6px;
		font-size: 13px;
		color: var(--text-dim);
	}
</style>
