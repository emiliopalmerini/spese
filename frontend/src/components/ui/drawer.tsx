import type * as React from "react"
import { Drawer as DrawerPrimitive } from "vaul"
import { XIcon } from "lucide-react"

import { cn } from "@/lib/utils"

const Drawer = DrawerPrimitive.Root
const DrawerTrigger = DrawerPrimitive.Trigger
const DrawerClose = DrawerPrimitive.Close

function DrawerContent({ className, children, ...props }: React.ComponentProps<typeof DrawerPrimitive.Content>) {
  return (
    <DrawerPrimitive.Portal>
      <DrawerPrimitive.Overlay className="fixed inset-0 z-50 bg-foreground/35" />
      <DrawerPrimitive.Content
        className={cn("fixed inset-x-0 bottom-0 z-50 mt-24 flex max-h-[94dvh] flex-col rounded-t-[24px] border-t bg-card outline-none", className)}
        {...props}
      >
        <div className="mx-auto my-3 h-1.5 w-12 rounded-full bg-border" />
        {children}
        <DrawerPrimitive.Close className="pressable absolute right-3 top-3 inline-flex size-11 items-center justify-center rounded-full hover:bg-secondary">
          <XIcon />
          <span className="sr-only">Chiudi</span>
        </DrawerPrimitive.Close>
      </DrawerPrimitive.Content>
    </DrawerPrimitive.Portal>
  )
}

function DrawerHeader({ className, ...props }: React.ComponentProps<"div">) {
  return <div className={cn("space-y-1 px-5 pb-4 text-left", className)} {...props} />
}

function DrawerTitle({ className, ...props }: React.ComponentProps<typeof DrawerPrimitive.Title>) {
  return <DrawerPrimitive.Title className={cn("font-display text-2xl font-semibold", className)} {...props} />
}

function DrawerDescription({ className, ...props }: React.ComponentProps<typeof DrawerPrimitive.Description>) {
  return <DrawerPrimitive.Description className={cn("text-sm text-muted-foreground", className)} {...props} />
}

export { Drawer, DrawerClose, DrawerContent, DrawerDescription, DrawerHeader, DrawerTitle, DrawerTrigger }
