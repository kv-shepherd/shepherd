import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string, options?: { name?: string; ns?: string }) => {
      const labels: Record<string, string> = {
        "console.chooser_title": "Choose console mode",
        "console.chooser_subtitle":
          "Select how to connect to {{name}}. Your confirmed choice will be reused next time.",
        "console.option_serial_title": "Serial Console",
        "console.option_serial_description":
          "Best for headless servers and SSH-first workloads. Use this when you need a reliable break-glass console.",
        "console.option_vnc_title": "noVNC Console",
        "console.option_vnc_description":
          "Use the graphical console when a graphics device is attached. Server images may still appear blank if the guest does not output to a graphical display.",
        "console.serial_available": "Serial available",
        "console.serial_disabled": "Serial disabled",
        "console.vnc_available": "Graphics available",
        "console.graphics_disabled": "Graphics disabled",
        "console.remote_access_note": "Daily remote access",
        "console.vnc_guest_output_hint":
          "noVNC needs both a graphics device and guest display output. Headless server images may still show a blank screen.",
        "field.remote_access": "Remote Access",
        "remote_access.ssh_help":
          "Use SSH for routine Linux server access when the VM network is reachable.",
        "console.chooser_default_hint":
          "Your last confirmed choice becomes the default next time.",
        "common:button.confirm": "Confirm",
        "common:button.cancel": "Cancel",
      };
      let message = labels[key] ?? key;
      if (options?.name) {
        message = message.replace("{{name}}", options.name);
      }
      return message;
    },
  }),
}));

import { ConsoleModeModal } from "./ConsoleModeModal";

describe("ConsoleModeModal", () => {
  it("disables unavailable console types and emits the selected choice", () => {
    const onChange = vi.fn();
    const onConfirm = vi.fn();

    render(
      <ConsoleModeModal
        open={true}
        vmName="vm-alpha"
        vm={{
          ip_address: "10.0.0.18",
          os_family: "linux",
        }}
        capabilities={{
          serial_available: true,
          vnc_available: false,
        }}
        value="SERIAL"
        onCancel={vi.fn()}
        onChange={onChange}
        onConfirm={onConfirm}
      />,
    );

    expect(screen.getByText("Choose console mode")).toBeInTheDocument();
    expect(
      screen.getByText(
        "Select how to connect to vm-alpha. Your confirmed choice will be reused next time.",
      ),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("radio", {
        name: /noVNC Console Use the graphical console when a graphics device is attached\./,
      }),
    ).toBeDisabled();
    expect(screen.getByText("Daily remote access")).toBeInTheDocument();
    expect(screen.getByText("ssh <username>@10.0.0.18")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Confirm" }));
    expect(onConfirm).toHaveBeenCalled();
  });
});
