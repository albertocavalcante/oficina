<script lang="ts">
	import '../app.css';
	let { children } = $props();
</script>

<svelte:head>
	<title>oficina</title>
</svelte:head>

<div class="shell">
	<nav>
		<a href="/" class="logo">oficina</a>
		<div class="links">
			<a href="/">Jobs</a>
			<a href="/agents">Agents</a>
		</div>
	</nav>
	<main>
		{@render children()}
	</main>
</div>

<style>
	.shell {
		max-width: 960px;
		margin: 0 auto;
		padding: 0 16px;
	}
	nav {
		display: flex;
		align-items: center;
		gap: 24px;
		padding: 16px 0;
		border-bottom: 1px solid var(--border);
		margin-bottom: 24px;
	}
	.logo {
		font-size: 20px;
		font-weight: 700;
		color: var(--text);
	}
	.logo:hover {
		text-decoration: none;
	}
	.links {
		display: flex;
		gap: 16px;
	}
	main {
		padding-bottom: 48px;
	}
</style>
