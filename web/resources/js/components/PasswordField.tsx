import { Eye, EyeOff } from "lucide-react";
import { type ReactNode, useState } from "react";

import FieldErrors from "@/components/FieldErrors";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useT } from "@/lib/i18n";

// A password field with the eye that shows and hides what was typed (SDD-012 section 2.1.1). The eye is a
// button with a name and a pressed state, in the tab order, so a keyboard and a reader get it too. The
// reasons the server gave for the field are written under it.
export default function PasswordField({
	id,
	label,
	value,
	onChange,
	autoComplete,
	reasons = [],
}: {
	id: string;
	label: string;
	value: string;
	onChange(next: string): void;
	autoComplete: "new-password" | "current-password";
	reasons?: readonly string[];
}): ReactNode {
	const t = useT();
	const [shown, setShown] = useState(false);
	const invalid = reasons.length > 0;
	return (
		<div className="flex flex-col gap-2">
			<Label htmlFor={id}>{label}</Label>
			<div className="flex items-center gap-2">
				<Input
					id={id}
					type={shown ? "text" : "password"}
					required
					autoComplete={autoComplete}
					spellCheck={false}
					autoCapitalize="none"
					aria-invalid={invalid ? true : undefined}
					aria-describedby={invalid ? `${id}-errors` : undefined}
					value={value}
					onChange={(event) => onChange(event.target.value)}
				/>
				<Button
					type="button"
					variant="ghost"
					size="icon"
					aria-pressed={shown}
					aria-label={shown ? t("ui.account.hide") : t("ui.account.show")}
					data-testid={`${id}-eye`}
					onClick={() => setShown(!shown)}
				>
					{shown ? <EyeOff aria-hidden="true" /> : <Eye aria-hidden="true" />}
				</Button>
			</div>
			<FieldErrors id={`${id}-errors`} reasons={reasons} />
		</div>
	);
}
