type ChannelCreateDialogProps = {
  // Called after a successful create with the new (server-assigned) channel id, so the
  // caller can navigate to it. The dialog itself does not route — the list owns navigation.
  onCreated: (id: string) => void;
  onClose: () => void;
};

export type { ChannelCreateDialogProps };
