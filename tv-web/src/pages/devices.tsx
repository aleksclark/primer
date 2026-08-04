import { useState } from "react";
import { Ban, KeyRound, Plus, RotateCw } from "lucide-react";
import type { components } from "@/api/schema";
import { ResourcePage } from "@/components/resource-page";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { mutate } from "@/api/client";
import { badgeCol, dateTimeCol, textCol } from "@/lib/columns";
import { DEVICE_KINDS } from "@/lib/constants";
import { formatDateTime } from "@/lib/datetime";

type Device = components["schemas"]["Device"];

/**
 * DevicesPage manages paired clients. Registration is a bespoke server
 * operation because the server mints the pairing code, so creation goes
 * through its own dialog rather than the generic create form.
 */
export function DevicesPage() {
  const [registering, setRegistering] = useState(false);
  const [issued, setIssued] = useState<Device | null>(null);
  const [reloads, setReloads] = useState(0);

  /** regenerate re-issues a pairing code, which also revokes the old token. */
  const regenerate = async (device: Device) => {
    if (device.revokedAt) {
      // A revoked device rejects pairing, so a fresh code would be dead on
      // arrival. Say so instead of handing over a code that cannot work.
      alert(`“${device.name}” is revoked and cannot pair. Register it again instead.`);
      return;
    }
    if (!confirm(`Issue a new pairing code for “${device.name}”? It will have to pair again.`)) return;
    setIssued((await mutate("POST", `/devices/${device.id}/pairing-code`)) as Device);
  };

  /** revoke marks a device revoked, which invalidates its token server-side. */
  const revoke = async (device: Device) => {
    if (device.revokedAt) return;
    if (!confirm(`Revoke “${device.name}”? Its token stops working immediately.`)) return;
    await mutate("PATCH", `/devices/${device.id}`, { revokedAt: new Date().toISOString() });
  };

  return (
    <>
      <ResourcePage<Device>
        refreshToken={reloads}
        title="Devices"
        path="/devices"
        description="Paired tablets and TV boxes. A device holds a token issued by redeeming a pairing code; revoking or re-issuing a code invalidates it."
        defaultSort="created_at"
        canCreate={false}
        toolbar={
          <Button variant="outline" onClick={() => setRegistering(true)}>
            <Plus /> Register device
          </Button>
        }
        rowActions={[
          { label: "New pairing code", icon: RotateCw, run: regenerate },
          { label: "Revoke", icon: Ban, run: revoke },
        ]}
        columns={[
          textCol<Device>("name", "Name", (r) => r.name, { sortable: true }),
          badgeCol<Device>("kind", "Kind", (r) => r.kind, { sortable: true }),
          {
            key: "pairingCode",
            header: "Pairing code",
            render: (row) => <PairingCode device={row} />,
          },
          {
            key: "status",
            header: "Status",
            render: (row) => <DeviceStatus device={row} />,
          },
          dateTimeCol<Device>("last_seen_at", "Last seen", (r) => r.lastSeenAt, { sortable: true }),
        ]}
        fields={[
          { key: "name", label: "Name" },
          { key: "kind", label: "Kind", type: "select", options: DEVICE_KINDS },
          {
            key: "revokedAt",
            label: "Revoked at",
            type: "datetime",
            help: "Set to revoke the device's token. Revoking cannot be undone from here — register the device again to bring it back.",
          },
        ]}
      />

      <RegisterDialog
        open={registering}
        onClose={() => setRegistering(false)}
        onRegistered={(device) => {
          setIssued(device);
          setReloads((n) => n + 1);
        }}
      />

      <PairingCodeDialog
        device={issued}
        onClose={() => {
          setIssued(null);
          setReloads((n) => n + 1);
        }}
      />
    </>
  );
}

/** PairingCode shows an outstanding code, or why there is not one. */
function PairingCode({ device }: { device: Device }) {
  if (!device.pairingCode) {
    return <span className="type-label text-muted-foreground">—</span>;
  }
  const expired =
    device.pairingExpiresAt != null && new Date(device.pairingExpiresAt).getTime() <= Date.now();
  return (
    <div className="space-y-0.5">
      <code
        className={
          expired
            ? "font-mono text-muted-foreground line-through tracking-widest"
            : "font-mono font-medium tracking-widest"
        }
      >
        {device.pairingCode}
      </code>
      {device.pairingExpiresAt && (
        <div className="type-label text-muted-foreground">
          {expired ? "expired" : `expires ${formatDateTime(device.pairingExpiresAt)}`}
        </div>
      )}
    </div>
  );
}

/** DeviceStatus summarises pairing and revocation state. */
function DeviceStatus({ device }: { device: Device }) {
  if (device.revokedAt) return <Badge variant="destructive">Revoked</Badge>;
  if (device.pairedAt) return <Badge variant="secondary">Paired</Badge>;
  return <Badge variant="outline">Awaiting pairing</Badge>;
}

interface RegisterDialogProps {
  open: boolean;
  onClose: () => void;
  onRegistered: (device: Device) => void;
}

/** RegisterDialog creates a device, which mints its first pairing code. */
function RegisterDialog({ open, onClose, onRegistered }: RegisterDialogProps) {
  const [error, setError] = useState<string | null>(null);

  const submit = async (form: FormData) => {
    setError(null);
    try {
      const device = (await mutate("POST", "/devices", {
        name: String(form.get("name") ?? ""),
        kind: String(form.get("kind") ?? "") || undefined,
      })) as Device;
      onClose();
      onRegistered(device);
    } catch (err) {
      setError((err as Error).message);
    }
  };

  return (
    <Dialog open={open} onOpenChange={(next) => !next && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Register device</DialogTitle>
        </DialogHeader>
        <form
          className="space-y-4"
          onSubmit={(e) => {
            e.preventDefault();
            void submit(new FormData(e.currentTarget));
          }}
        >
          <div className="space-y-2">
            <Label htmlFor="device-name">Name</Label>
            <Input id="device-name" name="name" required placeholder="Living room TV box" />
          </div>
          <div className="space-y-2">
            <Label htmlFor="device-kind">Kind</Label>
            <select
              id="device-kind"
              name="kind"
              defaultValue="tablet"
              className="flex h-10 w-full rounded-none border border-input bg-surface-raised px-3.5 py-3 text-sm text-foreground focus-visible:border-primary focus-visible:outline focus-visible:outline-[length:var(--primer-focus-width)] focus-visible:outline-offset-[var(--primer-focus-offset)] focus-visible:outline-primary"
            >
              {DEVICE_KINDS.map((kind) => (
                <option key={kind} value={kind}>
                  {kind}
                </option>
              ))}
            </select>
          </div>
          {error && (
            <p className="type-label text-attention" role="alert">
              {error}
            </p>
          )}
          <DialogFooter>
            <Button type="submit">Register</Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

/**
 * PairingCodeDialog surfaces a freshly minted code large enough to read off the
 * screen while typing it into the client.
 */
function PairingCodeDialog({ device, onClose }: { device: Device | null; onClose: () => void }) {
  return (
    <Dialog open={device !== null} onOpenChange={(next) => !next && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Pairing code</DialogTitle>
        </DialogHeader>
        {device && (
          <div className="space-y-3 text-center">
            <p className="text-sm text-muted-foreground">
              Enter this code on <span className="font-medium text-foreground">{device.name}</span>.
            </p>
            <p className="inline-flex items-center gap-2 font-mono text-4xl font-semibold tracking-[0.3em] text-foreground">
              <KeyRound className="h-6 w-6 text-muted-foreground" />
              {device.pairingCode}
            </p>
            {device.pairingExpiresAt && (
              <p className="type-label text-muted-foreground">
                Expires {formatDateTime(device.pairingExpiresAt)}.
              </p>
            )}
          </div>
        )}
        <DialogFooter>
          <Button onClick={onClose}>Done</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
