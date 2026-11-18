"use client"

import * as React from "react"
import { cn } from "@/lib/cn"
import { Dialog as DialogPrimitive } from "@base-ui/react/dialog"
import { Button } from "@/components/ui/button"
import { childRender } from "@/components/ui/slot"
import { XIcon } from "lucide-react"
import { useT } from "@/lib/i18n"

const DialogEscapeContext = React.createContext<React.MutableRefObject<
  ((event: KeyboardEvent) => void) | null
> | null>(null)

function Dialog({
  onOpenChange,
  children,
  ...props
}: React.ComponentProps<typeof DialogPrimitive.Root>) {
  const escape = React.useRef<((event: KeyboardEvent) => void) | null>(null)
  return (
    <DialogEscapeContext.Provider value={escape}>
      <DialogPrimitive.Root
        onOpenChange={(open, details) => {
          if (
            !open &&
            details.reason === "escape-key" &&
            escape.current &&
            details.event instanceof KeyboardEvent
          ) {
            escape.current(details.event)
            if (details.event.defaultPrevented) {
              details.cancel()
              return
            }
          }
          onOpenChange?.(open, details)
        }}
        {...props}
      >
        {children}
      </DialogPrimitive.Root>
    </DialogEscapeContext.Provider>
  )
}

function DialogTrigger({
  asChild,
  children,
  ...props
}: React.ComponentProps<typeof DialogPrimitive.Trigger> & {
  asChild?: boolean
}) {
  const render = childRender(asChild, children)
  return (
    <DialogPrimitive.Trigger
      data-slot="dialog-trigger"
      nativeButton={render ? false : undefined}
      render={render}
      {...props}
    >
      {render ? undefined : children}
    </DialogPrimitive.Trigger>
  )
}

function DialogPortal({
  ...props
}: React.ComponentProps<typeof DialogPrimitive.Portal>) {
  return <DialogPrimitive.Portal {...props} />
}

function DialogClose({
  asChild,
  children,
  ...props
}: React.ComponentProps<typeof DialogPrimitive.Close> & {
  asChild?: boolean
}) {
  const render = childRender(asChild, children)
  return (
    <DialogPrimitive.Close
      data-slot="dialog-close"
      nativeButton={render ? false : undefined}
      render={render}
      {...props}
    >
      {render ? undefined : children}
    </DialogPrimitive.Close>
  )
}

function DialogOverlay({
  className,
  ...props
}: React.ComponentProps<typeof DialogPrimitive.Backdrop>) {
  return (
    <DialogPrimitive.Backdrop
      data-slot="dialog-overlay"
      className={cn(
        "fixed inset-0 isolate z-50 bg-black/10 duration-100 supports-backdrop-filter:backdrop-blur-xs data-open:animate-in data-open:fade-in-0 data-closed:animate-out data-closed:fade-out-0",
        className
      )}
      {...props}
    />
  )
}

function DialogContent({
  className,
  children,
  showCloseButton = true,
  onEscapeKeyDown,
  ...props
}: React.ComponentProps<typeof DialogPrimitive.Popup> & {
  showCloseButton?: boolean
  onEscapeKeyDown?: (event: KeyboardEvent) => void
}) {
  const t = useT()
  const escape = React.useContext(DialogEscapeContext)
  React.useEffect(() => {
    if (!escape) return
    escape.current = onEscapeKeyDown ?? null
    return () => {
      escape.current = null
    }
  }, [escape, onEscapeKeyDown])
  return (
    <DialogPortal>
      <DialogOverlay />
      <DialogPrimitive.Popup
        data-slot="dialog-content"
        className={cn(
          "fixed top-1/2 left-1/2 z-50 grid w-full max-w-[calc(100%-2rem)] -translate-x-1/2 -translate-y-1/2 gap-4 rounded-xl bg-popover p-4 text-sm text-popover-foreground ring-1 ring-foreground/10 duration-100 outline-none sm:max-w-sm data-open:animate-in data-open:fade-in-0 data-open:zoom-in-95 data-closed:animate-out data-closed:fade-out-0 data-closed:zoom-out-95",
          className
        )}
        {...props}
      >
        {children}
        {showCloseButton && (
          <DialogPrimitive.Close
            data-slot="dialog-close"
            render={
              <Button
                variant="ghost"
                className="absolute top-2 right-2"
                size="icon-sm"
              />
            }
          >
            <XIcon />
            <span className="sr-only">{t("ui.editor.dialog.close")}</span>
          </DialogPrimitive.Close>
        )}
      </DialogPrimitive.Popup>
    </DialogPortal>
  )
}

function DialogHeader({ className, ...props }: React.ComponentProps<"div">) {
  return (
    <div
      data-slot="dialog-header"
      className={cn("flex flex-col gap-2", className)}
      {...props}
    />
  )
}

function DialogFooter({
  className,
  showCloseButton = false,
  children,
  ...props
}: React.ComponentProps<"div"> & {
  showCloseButton?: boolean
}) {
  const t = useT()
  return (
    <div
      data-slot="dialog-footer"
      className={cn(
        "-mx-4 -mb-4 flex flex-col-reverse gap-2 rounded-b-xl border-t bg-muted/50 p-4 sm:flex-row sm:justify-end",
        className
      )}
      {...props}
    >
      {children}
      {showCloseButton && (
        <DialogPrimitive.Close
          render={<Button variant="outline" />}
        >
          {t("ui.editor.dialog.close")}
        </DialogPrimitive.Close>
      )}
    </div>
  )
}

function DialogTitle({
  className,
  ...props
}: React.ComponentProps<typeof DialogPrimitive.Title>) {
  return (
    <DialogPrimitive.Title
      data-slot="dialog-title"
      className={cn(
        "font-heading text-base leading-none font-medium",
        className
      )}
      {...props}
    />
  )
}

function DialogDescription({
  className,
  ...props
}: React.ComponentProps<typeof DialogPrimitive.Description>) {
  return (
    <DialogPrimitive.Description
      data-slot="dialog-description"
      className={cn(
        "text-sm text-muted-foreground *:[a]:underline *:[a]:underline-offset-3 *:[a]:hover:text-foreground",
        className
      )}
      {...props}
    />
  )
}

export {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogOverlay,
  DialogPortal,
  DialogTitle,
  DialogTrigger,
}
