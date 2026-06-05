<script lang="ts">
  import { Combobox } from 'bits-ui'
  import { ChevronDown, Check, X } from 'lucide-svelte'
  import { untrack } from 'svelte'

  type BaseOption = { value: string; label: string }

  type BaseProps = {
    options: BaseOption[]
    placeholder?: string
    searchPlaceholder?: string
    emptyMessage?: string
    size?: 'xs' | 'sm'
    disabled?: boolean
  }

  type SingleProps = BaseProps & {
    type?: 'single'
    value: string
    allLabel?: never
    onValueChange?: (value: string) => void
  }

  type MultipleProps = BaseProps & {
    type: 'multiple'
    value: string[]
    allLabel?: string
    onValueChange?: (value: string[]) => void
  }

  type Props = SingleProps | MultipleProps

  let {
    type = 'single',
    options,
    value = $bindable(),
    placeholder = 'Select…',
    searchPlaceholder = 'Search…',
    emptyMessage = 'No results found.',
    size = 'sm',
    disabled = false,
    allLabel,
    onValueChange,
  }: Props = $props()

  let inputValue = $state('')
  let open = $state(false)
  let inputRef = $state<HTMLInputElement | null>(null)
  // Snapshot of which options were selected at the moment the dropdown opened.
  // We pin ordering to this snapshot so that toggling options doesn't make
  // items jump mid-interaction.
  let openSelectedSet = $state<Set<string>>(new Set())

  const filtered = $derived.by(() => {
    const q = inputValue.toLowerCase()
    const matches = q === ''
      ? options
      : options.filter((o) => o.label.toLowerCase().includes(q))
    if (type !== 'multiple' || openSelectedSet.size === 0) return matches
    const sel: BaseOption[] = []
    const rest: BaseOption[] = []
    for (const o of matches) {
      if (openSelectedSet.has(o.value)) sel.push(o)
      else rest.push(o)
    }
    return [...sel, ...rest]
  })

  const displayLabel = $derived.by(() => {
    if (type === 'multiple') {
      const vals = value as string[]
      if (vals.length === 0) return allLabel ?? placeholder
      if (vals.length === 1) {
        const match = options.find((o) => o.value === vals[0])
        return match?.label ?? vals[0]
      }
      return `${vals.length} selected`
    }
    const v = value as string
    if (!v) return placeholder
    const match = options.find((o) => o.value === v)
    return match?.label ?? v
  })

  const hasSelection = $derived(
    type === 'multiple' ? (value as string[]).length > 0 : !!(value as string),
  )

  const textSize = $derived(size === 'xs' ? 'text-xs' : 'text-sm')
  const iconSize = $derived(size === 'xs' ? 12 : 14)
  const itemPy = $derived(size === 'xs' ? 'py-1' : 'py-1.5')

  function selectAll() {
    value = [] as any
    open = false
    onValueChange?.([] as any)
  }

  // Snapshot selected options whenever the dropdown opens, regardless of whether
  // bits-ui or the parent toggled `open`. onOpenChange only fires for internal
  // toggles, so it misses opens triggered by Combobox.Input's onclick handler.
  // bits-ui's toggleItem also fills inputValue with the item's label after every
  // selection; we additionally null the DOM input.value so it can't carry over to
  // the next open even if Svelte hasn't propagated the state assignment yet.
  $effect(() => {
    if (open) {
      untrack(() => {
        inputValue = ''
        openSelectedSet = type === 'multiple' ? new Set(value as string[]) : new Set()
        if (inputRef) {
          inputRef.value = ''
        }
      })
    }
  })
</script>

<Combobox.Root
  {type}
  bind:value={value as any}
  bind:open
  bind:inputValue
  onValueChange={onValueChange as any}
  onOpenChange={(o) => { if (!o) inputValue = '' }}
  onOpenChangeComplete={(o) => { if (!o) inputValue = '' }}
  {disabled}
>
  <div class="relative">
    <Combobox.Input
      bind:ref={inputRef}
      oninput={(e) => (inputValue = e.currentTarget.value)}
      onfocus={(e) => {
        inputValue = ''
        ;(e.currentTarget as HTMLInputElement).value = ''
      }}
      onclick={(e) => {
        inputValue = ''
        ;(e.currentTarget as HTMLInputElement).value = ''
        if (!open) open = true
      }}
      class="flex items-center gap-1 w-full bg-bg text-fg border border-border rounded
        px-2 {itemPy} {textSize} placeholder:text-muted
        hover:bg-surface-hover focus:outline-none focus:ring-1 focus:ring-accent
        transition-colors disabled:opacity-50 disabled:cursor-not-allowed pr-7
        {open ? '' : 'text-transparent caret-transparent'}"
      placeholder={open ? searchPlaceholder : ''}
      aria-label={placeholder}
      {disabled}
    />
    {#if !open}
      <span
        class="absolute inset-0 flex items-center px-2 {textSize} text-fg pointer-events-none
          {hasSelection ? '' : 'text-muted'}"
      >{displayLabel}</span>
    {/if}
    {#if type === 'multiple' && (value as string[]).length > 0 && !open}
      <button
        type="button"
        class="absolute end-7 top-1/2 -translate-y-1/2 text-muted hover:text-fg transition-colors"
        onclick={(e) => { e.stopPropagation(); value = [] as any; onValueChange?.([] as any) }}
        aria-label="Clear all"
      >
        <X size={iconSize} />
      </button>
    {/if}
    <Combobox.Trigger
      class="absolute end-1.5 top-1/2 -translate-y-1/2 text-muted"
      {disabled}
    >
      <ChevronDown size={iconSize} />
    </Combobox.Trigger>
  </div>

  <Combobox.Portal>
    <Combobox.Content
      class="border border-border bg-bg rounded shadow-lg z-50 py-1
        max-h-[min(20rem,var(--bits-combobox-content-available-height))]
        w-[var(--bits-combobox-anchor-width)] min-w-[var(--bits-combobox-anchor-width)]
        select-none overflow-hidden"
      sideOffset={4}
    >
      <Combobox.Viewport class="p-0.5">
        {#if type === 'multiple' && allLabel}
          <button
            type="button"
            onclick={selectAll}
            class="flex items-center gap-2 w-full rounded px-2 {itemPy} {textSize}
              text-fg cursor-pointer outline-none
              hover:bg-surface-hover transition-colors"
          >
            <span
              class="shrink-0 w-3.5 h-3.5 rounded border flex items-center justify-center transition-colors
                {!hasSelection ? 'bg-accent border-accent text-bg' : 'border-muted'}"
            >
              {#if !hasSelection}<Check size={10} />{/if}
            </span>
            <span class="flex-1 truncate">{allLabel}</span>
          </button>
          <div class="border-t border-border my-0.5 mx-1"></div>
        {/if}
        {#each filtered as opt (opt.value)}
          <Combobox.Item
            value={opt.value}
            label={opt.label}
            class="flex items-center gap-2 w-full rounded px-2 {itemPy} {textSize}
              text-fg cursor-pointer outline-none
              data-[highlighted]:bg-surface-hover transition-colors"
          >
            {#snippet children({ selected })}
              {#if type === 'multiple'}
                <span
                  class="shrink-0 w-3.5 h-3.5 rounded border flex items-center justify-center transition-colors
                    {selected ? 'bg-accent border-accent text-bg' : 'border-muted'}"
                >
                  {#if selected}<Check size={10} />{/if}
                </span>
              {/if}
              <span class="flex-1 truncate">{opt.label}</span>
              {#if type === 'single' && selected}
                <Check size={iconSize} class="shrink-0 text-accent" />
              {/if}
            {/snippet}
          </Combobox.Item>
        {:else}
          <span class="block px-3 py-2 {textSize} text-muted">{emptyMessage}</span>
        {/each}
      </Combobox.Viewport>
    </Combobox.Content>
  </Combobox.Portal>
</Combobox.Root>
