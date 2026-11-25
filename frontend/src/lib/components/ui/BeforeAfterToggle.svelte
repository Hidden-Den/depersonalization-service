<script lang="ts">
  interface Props {
    /** true — исходные данные с ПДн, false — очищенные. */
    before: boolean;
  }

  let { before = $bindable() }: Props = $props();

  const options = [
    { value: false, label: 'Очищенные данные' },
    { value: true, label: 'Исходные с ПДн' }
  ];
</script>

<div class="h-7 inline-flex items-center border border-[#E8E7E3] bg-[#F0EFEB] rounded-md p-0.5 text-xs font-medium" role="group">
  {#each options as opt}
    <button
      type="button"
      onclick={() => (before = opt.value)}
      aria-pressed={before === opt.value}
      class="h-full px-2.5 sm:px-3 rounded pressable flex items-center transition cursor-pointer {before === opt.value
        ? 'bg-white text-[#1C1917] font-semibold shadow-xs'
        : 'text-[#78716C] hover:text-[#1C1917]'}"
    >
      {opt.label}
    </button>
  {/each}
</div>
