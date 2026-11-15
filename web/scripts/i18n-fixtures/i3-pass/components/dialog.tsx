declare const t: (key: string) => string;

// Words go through t(); class names, test ids and format examples are not words of a language.
export const A = () => <span className="sr-only">{t("ui.editor.dialog.close")}</span>;
export const B = () => <input data-testid="name-field" role="searchbox" placeholder="sha256:…" />;
export const C = () => <input placeholder="prj_..." />;
