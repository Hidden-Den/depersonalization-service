<script lang="ts">
  interface Props {
    id: string;
    label: string;
    value: string;
    placeholder: string;
    onBrowse: () => void;
  }

  let { id, label, value = $bindable(), placeholder, onBrowse }: Props = $props();
</script>

<div class="space-y-1.5">
  <label for={id} class="block text-xs font-semibold text-[#1C1917]">{label}</label>
  <div class="flex items-center gap-2">
    <input
      {id}
      type="text"
      bind:value
      {placeholder}
      class="flex-1 px-3 py-1.5 border border-[#E8E7E3] rounded-md focus:border-[#1C1917] focus:outline-none text-xs font-mono bg-[#FAF9F5] truncate"
    />
    <button
      type="button"
      onclick={onBrowse}
      class="px-3 py-1.5 bg-white border border-[#E8E7E3] hover:border-[#1C1917] text-[#1C1917] text-xs font-semibold rounded-md whitespace-nowrap cursor-pointer shadow-2xs pressable"
    >
      Обзор...
    </button>
  </div>
</div>
