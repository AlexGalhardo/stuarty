declare const t: (key: string) => string;
declare const code: string;

export const A = () => <p>{t("ui.editor.undo")}</p>;
export const B = () => <p>{t(`ui.editor.errors.${code}`)}</p>;
