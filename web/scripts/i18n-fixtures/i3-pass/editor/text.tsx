declare const t: (key: string, params?: Record<string, number>) => string;

// Words go through t(); symbols and numbers may stay.
export const A = () => <button aria-label={t("ui.editor.undo")}>{t("ui.editor.redo")}</button>;
export const B = () => <span>/ 12 · {t("ui.editor.revision", { revision: 3 })}</span>;
export const C = () => <input alt="" title={t("ui.editor.back")} />;
