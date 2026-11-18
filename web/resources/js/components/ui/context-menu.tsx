"use client"

import * as React from "react"
import { cn } from "@/lib/cn"
import {
  Menu,
  MenuCheckboxItem,
  MenuContent,
  MenuContextTrigger,
  MenuGroup,
  MenuGroupLabel,
  MenuItem,
  MenuItemIndicator,
  MenuRadioGroup,
  MenuRadioItem,
  MenuSeparator,
  MenuSub,
  MenuSubContent,
  MenuSubTrigger,
} from "@ui-sdk/react/menu"
import { childRender } from "@/components/ui/slot"
import { CheckIcon, ChevronRightIcon } from "lucide-react"

const itemClass =
  "group/context-menu-item relative flex cursor-default items-center gap-1.5 rounded-md px-1.5 py-1 text-sm outline-hidden select-none focus:bg-accent focus:text-accent-foreground data-highlighted:bg-accent data-highlighted:text-accent-foreground data-inset:pl-7 data-[variant=destructive]:text-destructive data-[variant=destructive]:focus:bg-destructive/10 data-[variant=destructive]:focus:text-destructive dark:data-[variant=destructive]:focus:bg-destructive/20 data-disabled:pointer-events-none data-disabled:opacity-50 [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*='size-'])]:size-4 data-[variant=destructive]:*:[svg]:text-destructive"

const popupClass =
  "z-50 max-h-(--available-height) min-w-36 origin-(--transform-origin) overflow-x-hidden overflow-y-auto rounded-lg bg-popover p-1 text-popover-foreground shadow-md ring-1 ring-foreground/10 duration-100 data-[side=bottom]:slide-in-from-top-2 data-[side=left]:slide-in-from-right-2 data-[side=right]:slide-in-from-left-2 data-[side=top]:slide-in-from-bottom-2 data-open:animate-in data-open:fade-in-0 data-open:zoom-in-95 data-closed:animate-out data-closed:fade-out-0 data-closed:zoom-out-95"

function ContextMenu({
  ...props
}: React.ComponentProps<typeof Menu>) {
  return <Menu {...props} />
}

function ContextMenuTrigger({
  asChild,
  className,
  children,
  ...props
}: Omit<React.ComponentProps<typeof MenuContextTrigger>, "children"> & {
  asChild?: boolean
  children?: React.ReactNode
}) {
  const render = childRender(asChild, children)
  return (
    <MenuContextTrigger
      data-slot="context-menu-trigger"
      className={cn("select-none", className)}
      render={render}
      {...props}
    >
      {render ? undefined : children}
    </MenuContextTrigger>
  )
}

function ContextMenuGroup({
  ...props
}: React.ComponentProps<typeof MenuGroup>) {
  return <MenuGroup data-slot="context-menu-group" {...props} />
}

function ContextMenuPortal({ children }: { children?: React.ReactNode }) {
  return <>{children}</>
}

function ContextMenuSub({
  ...props
}: React.ComponentProps<typeof MenuSub>) {
  return <MenuSub {...props} />
}

function ContextMenuRadioGroup({
  name,
  ...props
}: React.ComponentProps<typeof MenuRadioGroup>) {
  const fallback = React.useId()
  return (
    <MenuRadioGroup
      data-slot="context-menu-radio-group"
      name={name ?? fallback}
      {...props}
    />
  )
}

function ContextMenuContent({
  className,
  ...props
}: React.ComponentProps<typeof MenuContent>) {
  return (
    <MenuContent
      data-slot="context-menu-content"
      className={cn(popupClass, className)}
      {...props}
    />
  )
}

function ContextMenuItem({
  className,
  inset,
  variant = "default",
  ...props
}: React.ComponentProps<typeof MenuItem> & {
  inset?: boolean
  variant?: "default" | "destructive"
}) {
  return (
    <MenuItem
      data-slot="context-menu-item"
      data-inset={inset ? "" : undefined}
      data-variant={variant}
      destructive={variant === "destructive"}
      className={cn(itemClass, className)}
      {...props}
    />
  )
}

function ContextMenuSubTrigger({
  className,
  inset,
  children,
  ...props
}: Omit<React.ComponentProps<typeof MenuSubTrigger>, "children"> & {
  inset?: boolean
  children?: React.ReactNode
}) {
  return (
    <MenuSubTrigger
      data-slot="context-menu-sub-trigger"
      data-inset={inset ? "" : undefined}
      className={cn(
        "flex cursor-default items-center gap-1.5 rounded-md px-1.5 py-1 text-sm outline-hidden select-none focus:bg-accent focus:text-accent-foreground data-highlighted:bg-accent data-highlighted:text-accent-foreground data-inset:pl-7 data-open:bg-accent data-open:text-accent-foreground [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*='size-'])]:size-4",
        className
      )}
      {...props}
    >
      {children}
      <ChevronRightIcon className="ml-auto" />
    </MenuSubTrigger>
  )
}

function ContextMenuSubContent({
  className,
  ...props
}: React.ComponentProps<typeof MenuSubContent>) {
  return (
    <MenuSubContent
      data-slot="context-menu-sub-content"
      className={cn(popupClass, "min-w-32 border shadow-lg", className)}
      {...props}
    />
  )
}

function ContextMenuCheckboxItem({
  className,
  children,
  checked,
  inset,
  value,
  ...props
}: Omit<React.ComponentProps<typeof MenuCheckboxItem>, "children"> & {
  inset?: boolean
  children?: React.ReactNode
}) {
  const fallback = React.useId()
  return (
    <MenuCheckboxItem
      data-slot="context-menu-checkbox-item"
      data-inset={inset ? "" : undefined}
      className={cn(
        "relative flex cursor-default items-center gap-1.5 rounded-md py-1 pr-8 pl-1.5 text-sm outline-hidden select-none focus:bg-accent focus:text-accent-foreground data-highlighted:bg-accent data-highlighted:text-accent-foreground data-inset:pl-7 data-disabled:pointer-events-none data-disabled:opacity-50 [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*='size-'])]:size-4",
        className
      )}
      value={value ?? fallback}
      checked={checked}
      {...props}
    >
      <span className="pointer-events-none absolute right-2">
        <MenuItemIndicator keepMounted>
          <CheckIcon />
        </MenuItemIndicator>
      </span>
      {children}
    </MenuCheckboxItem>
  )
}

function ContextMenuRadioItem({
  className,
  children,
  inset,
  ...props
}: Omit<React.ComponentProps<typeof MenuRadioItem>, "children"> & {
  inset?: boolean
  children?: React.ReactNode
}) {
  return (
    <MenuRadioItem
      data-slot="context-menu-radio-item"
      data-inset={inset ? "" : undefined}
      className={cn(
        "relative flex cursor-default items-center gap-1.5 rounded-md py-1 pr-8 pl-1.5 text-sm outline-hidden select-none focus:bg-accent focus:text-accent-foreground data-highlighted:bg-accent data-highlighted:text-accent-foreground data-inset:pl-7 data-disabled:pointer-events-none data-disabled:opacity-50 [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*='size-'])]:size-4",
        className
      )}
      {...props}
    >
      <span className="pointer-events-none absolute right-2">
        <MenuItemIndicator keepMounted>
          <CheckIcon />
        </MenuItemIndicator>
      </span>
      {children}
    </MenuRadioItem>
  )
}

function ContextMenuLabel({
  className,
  inset,
  ...props
}: React.ComponentProps<typeof MenuGroupLabel> & { inset?: boolean }) {
  return (
    <MenuGroupLabel
      data-slot="context-menu-label"
      data-inset={inset ? "" : undefined}
      className={cn(
        "px-1.5 py-1 text-xs font-medium text-muted-foreground data-inset:pl-7",
        className
      )}
      {...props}
    />
  )
}

function ContextMenuSeparator({
  className,
  ...props
}: React.ComponentProps<typeof MenuSeparator>) {
  return (
    <MenuSeparator
      data-slot="context-menu-separator"
      className={cn("-mx-1 my-1 h-px bg-border", className)}
      {...props}
    />
  )
}

function ContextMenuShortcut({
  className,
  ...props
}: React.ComponentProps<"span">) {
  return (
    <span
      data-slot="context-menu-shortcut"
      className={cn(
        "ml-auto text-xs tracking-widest text-muted-foreground group-focus/context-menu-item:text-accent-foreground",
        className
      )}
      {...props}
    />
  )
}

export {
  ContextMenu,
  ContextMenuTrigger,
  ContextMenuContent,
  ContextMenuItem,
  ContextMenuCheckboxItem,
  ContextMenuRadioItem,
  ContextMenuLabel,
  ContextMenuSeparator,
  ContextMenuShortcut,
  ContextMenuGroup,
  ContextMenuPortal,
  ContextMenuSub,
  ContextMenuSubContent,
  ContextMenuSubTrigger,
  ContextMenuRadioGroup,
}
