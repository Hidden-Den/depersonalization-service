<script lang="ts">
  import type { Snippet } from 'svelte';

  interface Props {
    id: string;
    checked: boolean;
    title: string;
    description: string;
    /** Дополнительные поля, видимые, пока флажок установлен. */
    children?: Snippet;
  }

  let { id, checked = $bindable(), title, description, children }: Props = $props();
</script>

<div class="space-y-3 border border-[#E8E7E3] rounded-lg p-3.5 bg-white hover-sheen transition">
  <label class="flex items-start space-x-2.5 cursor-pointer select-none group">
    <div class="relative flex items-center justify-center mt-0.5 shrink-0">
      <input {id} type="checkbox" bind:checked class="sr-only peer" />
      <div
        class="w-4 h-4 rounded border transition-colors flex items-center justify-center pressable peer-focus-visible:ring-2 peer-focus-visible:ring-[#1C1917]/30 {checked
          ? 'bg-[#1C1917] border-[#1C1917] checked'
          : 'bg-white border-[#A8A29E] group-hover:border-[#1C1917]'}"
      >
        <svg class="w-3 h-3 text-white" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
          <path class="checkmark-path" d="M5 13l4 4L19 7" />
        </svg>
      </div>
    </div>
    <div>
      <span class="font-semibold text-[#1C1917] text-xs">{title}</span>
      <div class="text-[11px] text-[#78716C] mt-0.5 leading-relaxed font-sans">{description}</div>
    </div>
  </label>

  {#if checked && children}
    <div class="pt-3 border-t border-[#F0EFEB] space-y-3">
      {@render children()}
    </div>
  {/if}
</div>
