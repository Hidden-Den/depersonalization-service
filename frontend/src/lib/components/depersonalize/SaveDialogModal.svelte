<script lang="ts">
  import { appState } from '../../stores/appState.svelte';
  import { tableState } from '../../stores/tableState.svelte';
  import {
    SaveFileDialog,
    SaveKeyFileDialog,
    SaveArchiveFileDialog,
    GenerateArchivePassword,
    ValidateArchivePassword,
    ProcessFileWithOptions,
    ValidateKeyPassword
  } from '../../api/wails';
  import { copyToClipboard } from '../../services/clipboard';
  import type { PasswordValidationResult } from '../../types/domain';
  import { getDefaultOutputPath } from '../../utils/path';
  import Modal from '../ui/Modal.svelte';
  import ModalHeader from '../ui/ModalHeader.svelte';
  import Icon from '../ui/Icon.svelte';
  import CheckboxCard from '../ui/CheckboxCard.svelte';
  import PathPicker from '../ui/PathPicker.svelte';
  import PasswordInput from '../ui/PasswordInput.svelte';

  interface Props {
    isOpen: boolean;
    onClose: () => void;
  }

  const { isOpen, onClose }: Props = $props();

  let salt = $state(appState.currentSalt || '');
  let shuffleRows = $state(false);
  let createKey = $state(true);
  let keyPassword = $state('');
  // Пустой пароль допустим: бэкенд сгенерирует его автоматически.
  const EMPTY_PASSWORD_VALIDATION: PasswordValidationResult = {
    valid: true,
    score: 0,
    errors: [],
    minLength: false,
    hasUppercase: false,
    hasLowercase: false,
    hasDigit: false,
    hasSpecial: false,
    onlyLatin: false,
  };
  let passwordValidation = $state<PasswordValidationResult>(EMPTY_PASSWORD_VALIDATION);

  const STRENGTH_LEVELS = [
    { label: 'Слабый', text: 'text-[#DC2626]', bar: 'bg-[#DC2626]' },
    { label: 'Средний', text: 'text-[#D97706]', bar: 'bg-[#D97706]' },
    { label: 'Хороший', text: 'text-[#2563EB]', bar: 'bg-[#2563EB]' },
    { label: 'Надежный', text: 'text-[#16A34A]', bar: 'bg-[#16A34A]' }
  ];
  const strength = $derived(
    STRENGTH_LEVELS[Math.min(Math.max(passwordValidation.score, 1), STRENGTH_LEVELS.length) - 1]!
  );

  const PASSWORD_REQUIREMENTS = [
    ['minLength', '12+ символов'],
    ['onlyLatin', 'Только латиница'],
    ['hasUppercase', 'Заглавные буквы'],
    ['hasLowercase', 'Строчные буквы'],
    ['hasDigit', 'Цифры 0-9'],
    ['hasSpecial', 'Спецсимволы']
  ] as const;
  let outputPath = $state('');
  let keyPath = $state('');
  let selectedExportFormat = $state('xlsx');

  // Archive state
  let createArchive = $state(false);
  let archivePassword = $state('');
  let archivePath = $state('');

  // Правила паролей живут только в Go; интерфейс лишь показывает результат проверки.
  let archivePasswordError = $state('');

  $effect(() => {
    const pwd = archivePassword;
    if (!createArchive || !pwd.trim()) {
      archivePasswordError = '';
      return;
    }
    ValidateArchivePassword(pwd)
      .then((msg) => {
        if (pwd === archivePassword) archivePasswordError = msg;
      })
      .catch((err) => console.error('Failed to validate archive password:', err));
  });

  // Real-time password validation for key
  $effect(() => {
    const pwd = keyPassword;
    if (!pwd.trim()) {
      passwordValidation = EMPTY_PASSWORD_VALIDATION;
      return;
    }
    ValidateKeyPassword(pwd)
      .then((res) => {
        if (pwd === keyPassword) passwordValidation = res;
      })
      .catch((err) => console.error('Failed to validate key password:', err));
  });

  const isPasswordValid = $derived(
    appState.currentFileInfo?.fileType === 'pdf' ||
    !createKey ||
    !keyPassword.trim() ||
    passwordValidation.valid
  );

  const isFormValid = $derived(
    isPasswordValid &&
    (!createArchive || !archivePassword.trim() || archivePasswordError === '')
  );

  // Generate random salt on open if not already set
  $effect(() => {
    if (isOpen) {
      if (!salt || salt === '152fz-secure-salt') {
        generateRandomSalt();
      }
    }
  });

  // Initialize selected export format when file changes
  $effect(() => {
    if (appState.currentFileInfo) {
      const formats = supportedFormats;
      if (formats && formats.length > 0 && formats[0]) {
        selectedExportFormat = formats[0];
      } else {
        const ext = (appState.currentFileInfo.extension || 'xlsx').replace('.', '').toLowerCase();
        selectedExportFormat = ext === 'xls' || ext === 'xlsb' ? 'xlsx' : ext;
      }
      if (appState.currentFileInfo.fileType !== 'table') {
        shuffleRows = false;
      }
      if (appState.currentFileInfo.fileType === 'pdf') {
        createKey = false;
      }
    }
  });

  const supportedFormats = $derived.by(() => {
    const info = appState.currentFileInfo;
    if (!info) return ['xlsx'];
    if (info.supportedExportFormats && info.supportedExportFormats.length > 0) {
      return info.supportedExportFormats.map((f) => f.replace('.', '').toLowerCase());
    }
    const ext = (info.extension || 'xlsx').replace('.', '').toLowerCase();
    return ext === 'xls' || ext === 'xlsb' ? ['xlsx'] : [ext];
  });

  function handleSelectFormat(fmt: string) {
    selectedExportFormat = fmt;
    if (outputPath) {
      outputPath = outputPath.replace(/\.[^/.]+$/, `.${fmt}`);
    }
  }

  const formatHint = $derived.by(() => {
    const origExt = (appState.currentFileInfo?.extension || '').replace('.', '').toLowerCase();
    if (origExt === 'xls' || origExt === 'xlsb') {
      return 'Бинарный формат Excel конвертируется в открытый формат .xlsx.';
    }
    if (selectedExportFormat === origExt) {
      return '';
    }
    return `Данные будут сохранены в формат .${selectedExportFormat}.`;
  });

  function getFormatDisplayName(fmt: string): string {
    switch (fmt.toLowerCase()) {
      case 'xlsx':
        return 'Таблица Microsoft Excel (.xlsx)';
      case 'ods':
        return 'Таблица OpenDocument Spreadsheet (.ods)';
      case 'csv':
        return 'Текстовая таблица CSV (.csv)';
      case 'docx':
        return 'Документ Microsoft Word (.docx)';
      case 'odt':
        return 'Документ OpenDocument Text (.odt)';
      case 'pdf':
        return 'Документ Adobe PDF (.pdf)';
      default:
        return fmt.toUpperCase();
    }
  }

  function generateRandomSalt() {
    const arr = new Uint8Array(8);
    crypto.getRandomValues(arr);
    salt = Array.from(arr, (b) => b.toString(16).padStart(2, '0')).join('');
  }

  async function handleSelectOutputPath() {
    const ext = selectedExportFormat;
    const defaultPath = getDefaultOutputPath(appState.currentFilePath || '', ext);
    const chosen = await SaveFileDialog(defaultPath);
    if (chosen) {
      outputPath = chosen;
    }
  }

  async function handleSelectKeyPath() {
    const baseSource = outputPath.trim() || appState.currentFilePath || '';
    const defaultPath = getDefaultOutputPath(baseSource, 'depmap');
    const chosen = await SaveKeyFileDialog(defaultPath);
    if (chosen) {
      keyPath = chosen;
    }
  }

  async function handleSelectArchivePath() {
    const baseSource = outputPath.trim() || appState.currentFilePath || '';
    const defaultPath = getDefaultOutputPath(baseSource, 'zip');
    const chosen = await SaveArchiveFileDialog(defaultPath);
    if (chosen) {
      archivePath = chosen;
    }
  }

  async function handleGenerateArchivePassword() {
    try {
      const pwd = await GenerateArchivePassword();
      if (pwd) {
        archivePassword = pwd;
      }
    } catch (err) {
      console.error('Failed to generate archive password:', err);
    }
  }

  async function handleCopyArchivePassword() {
    if (!archivePassword) return;
    const ok = await copyToClipboard(archivePassword);
    if (ok) {
      appState.showToast('Пароль архива скопирован в буфер обмена!', 'success');
    }
  }

  async function handleStart() {
    if (!appState.currentFileInfo || !appState.currentFilePath || appState.isProcessing) return;
    if (!isFormValid) return;

    let destPath = outputPath.trim();
    if (destPath) {
      const curExt = destPath.split('.').pop()?.toLowerCase();
      if (curExt !== selectedExportFormat) {
        destPath = `${destPath.replace(/\.[^/.]+$/, '')}.${selectedExportFormat}`;
      }
    } else {
      destPath = getDefaultOutputPath(appState.currentFilePath, selectedExportFormat);
    }

    // Save chosen salt to appState
    appState.setSalt(salt);

    // Calculate total rows across sheets
    let totalRows = appState.currentFileInfo.rowCount || 0;
    if (appState.currentFileInfo.sheets && appState.currentFileInfo.sheets.length > 1) {
      totalRows = appState.currentFileInfo.sheets.reduce(
        (acc, s) => acc + (s.rowCount || 0),
        0
      );
    }

    // Close this save options modal
    onClose();

    // Start progress modal
    appState.startProgress(
      appState.processingMode === 'delete' ? 'Удаление ПДн' : 'Деперсонализация ПДн'
    );
    appState.updateProgress(0, 0, totalRows);

    try {
      // Build disabled columns arrays
      const activeSheetName =
        appState.currentFileInfo.sheets?.[tableState.activeSheetIndex]?.name || '';
      const globalDisabled = tableState.disabledColumnsBySheet[activeSheetName] || [];
      const disabledBySheet = tableState.disabledColumnsBySheet;

      const effectiveMode =
        appState.processingMode === 'delete' ? 'delete' : (appState.strategy || 'tokens');
      const effectiveShuffle =
        appState.currentFileInfo.fileType === 'table' ? shuffleRows : false;

      const isPdf = appState.currentFileInfo.fileType === 'pdf';
      const effectiveCreateKey = isPdf ? false : createKey;
      const effectiveKeyPassword = effectiveCreateKey ? keyPassword : '';
      const effectiveKeyPath = effectiveCreateKey
        ? (keyPath.trim() || getDefaultOutputPath(destPath, 'depmap'))
        : '';
      const effectiveArchivePath = createArchive
        ? (archivePath.trim() || getDefaultOutputPath(destPath, 'zip'))
        : '';

      const result = await ProcessFileWithOptions({
        inputPath: appState.currentFilePath,
        outputPath: destPath,
        mode: appState.processingMode,
        strategy: effectiveMode,
        salt,
        disabledCols: globalDisabled,
        disabledBySheet,
        createKey: effectiveCreateKey,
        keyPassword: effectiveKeyPassword,
        keyPath: effectiveKeyPath,
        shuffleRows: effectiveShuffle,
        createArchive,
        archivePassword: createArchive ? archivePassword.trim() : undefined,
        archivePath: effectiveArchivePath,
        pdfRedactionBoxes: isPdf ? (appState.pdfRedactionBoxes || []) : undefined,
        pdfRedactTerms: isPdf ? (appState.pdfRedactTerms || []) : undefined,
      });

      if (result && result.success) {
        appState.showSuccess(result);
        appState.showToast(
          appState.processingMode === 'delete'
            ? 'Удаление ПДн успешно завершено!'
            : 'Деперсонализация успешно завершена!',
          'success'
        );
      } else {
        appState.showToast(result?.error || 'Ошибка при деперсонализации', 'error');
      }
    } catch (err: any) {
      console.error('Error during depersonalization:', err);
      const errStr = String(err);
      if (errStr.includes('context canceled') || errStr.includes('canceled') || errStr.includes('отменена')) {
        appState.showToast('Операция отменена пользователем', 'info');
      } else {
        appState.showToast(`Ошибка деперсонализации: ${err}`, 'error');
      }
    } finally {
      appState.finishProgress();
    }
  }
</script>

{#if isOpen && appState.currentFileInfo}
  <Modal
    id="save-options-modal"
    labelledby="save-modal-title"
    {onClose}
    class="max-w-lg p-6 gap-4 max-h-[calc(100%-2rem)] overflow-y-auto custom-scrollbar"
  >
      <ModalHeader titleId="save-modal-title" title="Параметры сохранения" {onClose} />

      <div class="space-y-3.5 text-xs">
        <div class="p-3 bg-[#FAF9F5] border border-[#E8E7E3] rounded-lg flex items-center justify-between font-mono">
          <span class="font-bold text-[#1C1917] truncate flex-1 min-w-0" title={appState.currentFileInfo.name}>
            {appState.currentFileInfo.name}
          </span>
          <span class="text-[#78716C] shrink-0 ml-2">
            {#if appState.currentFileInfo.sheets && appState.currentFileInfo.sheets.length > 1}
              {appState.currentFileInfo.sheets.length} листов
            {:else if appState.currentFileInfo.fileType === 'text'}
              {(appState.currentFileInfo.paragraphCount || appState.currentFileInfo.rowCount || 0).toLocaleString('ru-RU')} абз.
            {:else if appState.currentFileInfo.fileType === 'pdf'}
              {appState.currentFileInfo.pageCount} стр.
            {:else}
              {(appState.currentFileInfo.rowCount || 0).toLocaleString('ru-RU')} строк
            {/if}
          </span>
        </div>

        {#if supportedFormats.length > 1}
          <div class="space-y-2 border border-[#E8E7E3] rounded-lg p-3.5 bg-white">
            <div class="text-xs font-semibold text-[#1C1917]">Формат сохранения</div>
            <div class="grid gap-2" style="grid-template-columns: repeat({supportedFormats.length}, minmax(0, 1fr));">
              {#each supportedFormats as fmt}
                <button
                  type="button"
                  onclick={() => handleSelectFormat(fmt)}
                  aria-pressed={selectedExportFormat === fmt}
                  class="pressable py-1.5 rounded-md border text-center text-xs font-semibold cursor-pointer transition {selectedExportFormat === fmt
                    ? 'border-[#1C1917] bg-[#F5F5F4] text-[#1C1917] shadow-2xs'
                    : 'border-[#E8E7E3] hover:border-[#1C1917] bg-white text-[#78716C] hover:text-[#1C1917]'}"
                >
                  {fmt.toUpperCase()}
                </button>
              {/each}
            </div>
            {#if formatHint}
              <div class="text-[11px] text-[#78716C] mt-1 font-sans">{formatHint}</div>
            {/if}
          </div>
        {:else if formatHint}
          <div class="px-3 py-2 bg-[#FAF9F5] border border-[#E8E7E3] rounded-lg text-[11px] text-[#78716C] font-sans">
            {formatHint}
          </div>
        {/if}

        <div class="border border-[#E8E7E3] rounded-lg p-3.5 bg-white">
          <PathPicker
            id="output-path-input"
            label="Путь для сохранения файла"
            bind:value={outputPath}
            placeholder="По умолчанию рядом с исходным файлом..."
            onBrowse={handleSelectOutputPath}
          />
        </div>

        {#if appState.currentFileInfo.fileType === 'table'}
          <CheckboxCard
            id="shuffle-rows-checkbox"
            bind:checked={shuffleRows}
            title="Перемешать строки"
            description="Случайный порядок записей для защиты от деанонимизации по позиции в реестре. Данные внутри строк расшифровываются корректно."
          />
        {/if}

        {#if appState.currentFileInfo.fileType !== 'pdf'}
          <CheckboxCard
            id="create-key-checkbox"
            bind:checked={createKey}
            title="Создать ключ деобезличивания"
            description="Позволяет в будущем восстановить исходные данные."
          >
            <PathPicker
              id="key-path-input"
              label="Путь для сохранения ключа"
              bind:value={keyPath}
              placeholder="Рядом с обработанным файлом..."
              onBrowse={handleSelectKeyPath}
            />

            <div class="space-y-1.5">
              <div class="flex items-center justify-between">
                <label for="key-password-input" class="block text-xs font-semibold text-[#1C1917]">Пароль ключа</label>
                {#if keyPassword.trim()}
                  <span class="text-[10px] font-mono font-medium {strength.text}">{strength.label}</span>
                {/if}
              </div>

              <PasswordInput
                id="key-password-input"
                bind:value={keyPassword}
                placeholder="Оставьте пустым для автогенерации"
                invalid={!!keyPassword.trim() && !passwordValidation.valid}
              />

              {#if keyPassword.trim()}
                <div class="grid grid-cols-4 gap-1 pt-0.5">
                  {#each [1, 2, 3, 4] as level}
                    <div class="h-1 rounded-full transition-colors {passwordValidation.score >= level ? strength.bar : 'bg-[#E7E5E4]'}"></div>
                  {/each}
                </div>

                <div class="mt-2 bg-[#FAF9F5] p-2.5 border border-[#E8E7E3] rounded-md text-[11px] font-sans">
                  <div class="font-medium text-[#44403C] mb-1.5 text-[10.5px]">Требования к паролю:</div>
                  <div class="grid grid-cols-2 gap-x-2 gap-y-1">
                    {#each PASSWORD_REQUIREMENTS as [key, text]}
                      {@const met = passwordValidation[key]}
                      <div class="flex items-center space-x-1.5 {met ? 'text-[#16A34A] font-medium' : 'text-[#78716C]'}">
                        <span class="w-3.5 h-3.5 flex items-center justify-center shrink-0">
                          {#if met}
                            <Icon name="check" class="w-3 h-3 text-[#16A34A]" strokeWidth={2.5} />
                          {:else}
                            <span class="w-1 h-1 rounded-full bg-stone-400"></span>
                          {/if}
                        </span>
                        <span>{text}</span>
                      </div>
                    {/each}
                  </div>

                  {#if !passwordValidation.onlyLatin}
                    <div class="mt-2 pt-1.5 border-t border-[#E8E7E3] text-[10.5px] text-[#DC2626] font-medium flex items-center space-x-1.5">
                      <Icon name="warning" class="w-3.5 h-3.5 text-[#DC2626] shrink-0" />
                      <span>Кириллица и пробелы запрещены — используйте только латиницу</span>
                    </div>
                  {/if}
                </div>
              {:else}
                <p class="text-[11px] text-[#78716C] leading-relaxed">
                  Минимум 12 символов: только латиница (A-Z, a-z), цифры и спецсимволы. Если оставить пустым — пароль будет сгенерирован автоматически.
                </p>
              {/if}
            </div>
          </CheckboxCard>
        {/if}

        <CheckboxCard
          id="create-archive-checkbox"
          bind:checked={createArchive}
          title="Упаковать в ZIP-архив с паролем"
          description="Создает защищенный паролем архив с обработанным файлом для отправки по почте."
        >
          <PathPicker
            id="archive-path-input"
            label="Путь для сохранения архива"
            bind:value={archivePath}
            placeholder="По умолчанию рядом с файлом (*.zip)..."
            onBrowse={handleSelectArchivePath}
          />

          <div class="space-y-1.5">
            <div class="flex items-center justify-between">
              <label for="archive-password-input" class="block text-xs font-semibold text-[#1C1917]">Пароль архива</label>
              {#if archivePassword.trim()}
                <span class="text-[10px] font-mono {archivePasswordError ? 'text-[#DC2626]' : 'text-[#16A34A]'}">
                  {archivePasswordError ? 'Некорректный пароль' : 'Пароль подходит'}
                </span>
              {/if}
            </div>

            <div class="flex items-center gap-1.5">
              <PasswordInput
                id="archive-password-input"
                bind:value={archivePassword}
                placeholder="Оставьте пустым для автогенерации (16 симв.)"
                invalid={!!archivePassword.trim() && !!archivePasswordError}
              />
              <button
                type="button"
                onclick={handleGenerateArchivePassword}
                class="px-2.5 py-1.5 bg-white border border-[#E8E7E3] hover:border-[#1C1917] text-[#1C1917] text-xs font-semibold rounded-md whitespace-nowrap cursor-pointer shadow-2xs flex items-center gap-1.5 pressable"
                title="Сгенерировать случайный пароль"
              >
                <Icon name="refresh" class="w-3.5 h-3.5 text-stone-600" />
                <span>Сгенерировать</span>
              </button>
              {#if archivePassword}
                <button
                  type="button"
                  onclick={handleCopyArchivePassword}
                  class="px-2.5 py-1.5 bg-white border border-[#E8E7E3] hover:border-[#1C1917] text-[#1C1917] text-xs font-semibold rounded-md whitespace-nowrap cursor-pointer shadow-2xs pressable"
                  title="Скопировать пароль архива"
                  aria-label="Скопировать пароль архива"
                >
                  <Icon name="copy" class="w-3.5 h-3.5 text-stone-600" />
                </button>
              {/if}
            </div>

            {#if archivePasswordError}
              <div class="text-[10.5px] text-[#DC2626] font-medium flex items-center space-x-1.5 mt-1">
                <Icon name="warning" class="w-3.5 h-3.5 text-[#DC2626] shrink-0" />
                <span class="first-letter:uppercase">{archivePasswordError}</span>
              </div>
            {:else}
              <p class="text-[11px] text-[#78716C] leading-relaxed font-sans">
                Минимум 4 символа (только латиница, цифры и знаки). Если оставить пустым — при сохранении сгенерируется надёжный пароль на 16 символов.
              </p>
            {/if}
          </div>
        </CheckboxCard>
      </div>

      <div class="flex items-center justify-end space-x-2 pt-3 border-t border-[#F0EFEB]">
        <button
          type="button"
          onclick={onClose}
          class="h-8 px-4 bg-white border border-[#E8E7E3] hover:border-[#1C1917] text-[#1C1917] text-xs font-semibold rounded-md pressable cursor-pointer flex items-center justify-center"
        >
          Отмена
        </button>
        <button
          type="button"
          onclick={handleStart}
          disabled={!isFormValid || appState.isProcessing}
          class="h-8 px-5 text-xs font-semibold rounded-md transition flex items-center justify-center gap-1.5 shadow-sm pressable {isFormValid && !appState.isProcessing ? 'bg-[#1C1917] hover:bg-[#292524] text-white cursor-pointer' : 'bg-[#E7E5E4] text-[#A8A29E] cursor-not-allowed border border-[#D6D3D1]'}"
        >
          <Icon name="download" class="w-3.5 h-3.5 text-stone-300 shrink-0" />
          <span>{appState.processingMode === 'delete' ? 'Начать удаление' : 'Начать обезличивание'}</span>
        </button>
      </div>
  </Modal>
{/if}
