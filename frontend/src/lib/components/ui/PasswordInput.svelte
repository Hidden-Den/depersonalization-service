<script lang="ts">
  import Icon from './Icon.svelte';

  interface Props {
    id: string;
    value: string;
    placeholder: string;
    invalid?: boolean;
    class?: string;
    onkeydown?: (e: KeyboardEvent) => void;
  }

  let {
    id,
    value = $bindable(),
    placeholder,
    invalid = false,
    class: inputClass = 'py-1.5',
    onkeydown
  }: Props = $props();

  let visible = $state(false);
</script>

<div class="relative flex-1 flex items-center">
  <input
    {id}
    type={visible ? 'text' : 'password'}
    bind:value
    {placeholder}
    {onkeydown}
    autocomplete="off"
    class="w-full pl-3 pr-8 border rounded-md focus:outline-none text-xs font-mono bg-[#FAF9F5] {inputClass} {invalid
      ? 'border-[#DC2626] focus:border-[#DC2626]'
      : 'border-[#E8E7E3] focus:border-[#1C1917]'}"
  />
  {#if value}
    <button
      type="button"
      onclick={() => (visible = !visible)}
      class="absolute right-2 text-[#78716C] hover:text-[#1C1917] p-1 select-none cursor-pointer"
      title={visible ? 'Скрыть пароль' : 'Показать пароль'}
      aria-label={visible ? 'Скрыть пароль' : 'Показать пароль'}
    >
      <Icon name={visible ? 'eye-off' : 'eye'} class="w-3.5 h-3.5" />
    </button>
  {/if}
</div>
