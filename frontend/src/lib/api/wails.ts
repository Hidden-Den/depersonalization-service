// Тонкий типизированный слой над привязками Wails v2.
// Go-структура internal/desktop.DesktopApp привязана через Bind и доступна как
// window.go.desktop.DesktopApp; рантайм Wails — как window.runtime.
// Сгенерированные wailsjs/ не используются: приложение собирается обычным `go build`
// без Wails CLI, поэтому сигнатуры ниже ведутся вручную и должны совпадать с Go.
//
// Моков нет намеренно: вне Wails любой вызов бэкенда отклоняется с явной ошибкой,
// чтобы поломка привязки не выглядела как успешная операция.
import type {
  AppInfo,
  AuditRecord,
  FileInfo,
  KeyFileInfo,
  PasswordValidationResult,
  PreviewRow,
  ProcessOptions,
  ProcessResult,
  RestoreResult
} from '../types/domain';

interface DesktopApp {
  GetAppInfo(): Promise<AppInfo>;
  SelectFile(): Promise<string>;
  InspectFile(path: string): Promise<FileInfo>;
  GetFileBase64(path: string): Promise<string>;
  GetSheetRows(
    path: string,
    sheetName: string,
    offset: number,
    limit: number,
    salt: string,
    disabledCols: number[],
    mode: string
  ): Promise<PreviewRow[]>;
  ProcessFileWithOptions(options: ProcessOptions): Promise<ProcessResult>;
  CancelCurrentOperation(): Promise<void>;
  SaveFileDialog(defaultName: string): Promise<string>;
  SelectKeyFile(): Promise<string>;
  SaveKeyFileDialog(defaultName: string): Promise<string>;
  SaveArchiveFileDialog(defaultName: string): Promise<string>;
  SaveAuditFileDialog(defaultName: string): Promise<string>;
  InspectKeyFile(keyPath: string, password: string): Promise<KeyFileInfo>;
  RestoreFileTo(inputPath: string, keyPath: string, password: string, destPath: string): Promise<RestoreResult>;
  OpenContainingFolder(targetPath: string): Promise<void>;
  GetAuditRecords(limit: number): Promise<AuditRecord[]>;
  ExportAuditLog(destPath: string): Promise<void>;
  SetClipboardText(text: string): Promise<boolean>;
  ValidateKeyPassword(password: string): Promise<PasswordValidationResult>;
  GenerateArchivePassword(): Promise<string>;
  /** Возвращает текст ошибки или пустую строку, если пароль подходит. */
  ValidateArchivePassword(password: string): Promise<string>;
}

interface WailsRuntime {
  EventsOn(eventName: string, callback: (...data: any[]) => void): () => void;
  OnFileDrop(callback: (x: number, y: number, paths: string[]) => void, useDropTarget: boolean): void;
  OnFileDropOff(): void;
}

declare global {
  interface Window {
    go?: { desktop?: { DesktopApp?: DesktopApp } };
    runtime?: WailsRuntime;
  }
}

function backend(): DesktopApp {
  const app = window.go?.desktop?.DesktopApp;
  if (!app) {
    throw new Error('Бэкенд приложения недоступен: интерфейс запущен вне Wails');
  }
  return app;
}

// Обёртка через async превращает отсутствие бэкенда в отклонённый Promise,
// который вызывающий код обрабатывает так же, как ошибку из Go.
function bind<K extends keyof DesktopApp>(method: K): DesktopApp[K] {
  return (async (...args: unknown[]) =>
    (backend()[method] as (...a: unknown[]) => unknown)(...args)) as DesktopApp[K];
}

export const GetAppInfo = bind('GetAppInfo');
export const SelectFile = bind('SelectFile');
export const InspectFile = bind('InspectFile');
export const GetFileBase64 = bind('GetFileBase64');
export const GetSheetRows = bind('GetSheetRows');
export const ProcessFileWithOptions = bind('ProcessFileWithOptions');
export const CancelCurrentOperation = bind('CancelCurrentOperation');
export const SaveFileDialog = bind('SaveFileDialog');
export const SelectKeyFile = bind('SelectKeyFile');
export const SaveKeyFileDialog = bind('SaveKeyFileDialog');
export const SaveArchiveFileDialog = bind('SaveArchiveFileDialog');
export const SaveAuditFileDialog = bind('SaveAuditFileDialog');
export const InspectKeyFile = bind('InspectKeyFile');
export const RestoreFileTo = bind('RestoreFileTo');
export const OpenContainingFolder = bind('OpenContainingFolder');
export const GetAuditRecords = bind('GetAuditRecords');
export const ExportAuditLog = bind('ExportAuditLog');
export const SetClipboardText = bind('SetClipboardText');
export const ValidateKeyPassword = bind('ValidateKeyPassword');
export const GenerateArchivePassword = bind('GenerateArchivePassword');
export const ValidateArchivePassword = bind('ValidateArchivePassword');

/** Подписка на событие бэкенда. Возвращает функцию отписки. */
export function EventsOn(eventName: string, callback: (...data: any[]) => void): () => void {
  if (!window.runtime) {
    console.warn(`[wails] рантайм недоступен, подписка на «${eventName}» не выполнена`);
    return () => {};
  }
  return window.runtime.EventsOn(eventName, callback);
}

/** Нативный drag-and-drop файлов с полными путями ОС. Возвращает функцию отписки. */
export function OnFileDrop(callback: (x: number, y: number, paths: string[]) => void): () => void {
  const runtime = window.runtime;
  if (!runtime) {
    console.warn('[wails] рантайм недоступен, перетаскивание файлов отключено');
    return () => {};
  }
  runtime.OnFileDrop(callback, false);
  return () => runtime.OnFileDropOff();
}
