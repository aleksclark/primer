import { useState, type FormEvent, type ReactNode } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, within } from "storybook/test";
import { StudentChecklistScreen, StudentOccurrenceScreen, StudentPairScreen, type ChecklistScreenItem, type RequestState, type StudentOccurrenceScreenProps } from "./StudentScreens";

const meta = {
  title: "Tasks/Pilot workflow",
  parameters: { chromatic: { modes: { desktop: { viewport: 1280 }, mobile: { viewport: 390 } } } },
} satisfies Meta;

export default meta;
type Story = StoryObj;

const noop = fn();
const occurrence = (args: StudentOccurrenceScreenProps) => <StudentOccurrenceScreen {...args} />;
const checklistItems = [
  { id: "fractions", title: "Practice fractions", description: "Complete problems 1 through 12 and show each reduction.", status: "Ready to start", href: "#fractions" },
  { id: "essay", title: "Revise history essay", description: "Finish the conclusion and check every citation.", status: "In progress", href: "#essay" },
  { id: "reading", title: "Read The Hobbit", description: "Read chapter 4 and mark two unfamiliar words.", status: "Sent to your parent", href: "#reading" },
  { id: "workbench", title: "Measure the workbench", description: "Measure again and record each dimension to the nearest eighth inch.", status: "Needs another try", href: "#workbench" },
  { id: "science", title: "Record plant growth", description: "Measure the seedling and add today’s observation.", status: "Approved", href: "#science" },
  { id: "map", title: "Label the river map", description: "Label the major rivers discussed this week.", status: "Skipped", href: "#map" },
  { id: "vocabulary", title: "Review vocabulary", description: "Define this week’s ten vocabulary words.", status: "Canceled", href: "#vocabulary" },
  { id: "cad", title: "Inspect the CAD model", description: "Open the model and check the mounting-hole spacing.", status: "Unavailable on this device", href: "#cad" },
];

function PairingStory({ initialState = "ready" as RequestState }) {
  const [code, setCode] = useState("");
  const [state, setState] = useState<RequestState>(initialState);
  const submit = (event: FormEvent) => {
    event.preventDefault();
    setState("loading");
  };
  const notice = state === "expired" ? <p className="notice expired" role="alert">This code has expired or already been used. Ask for a new code.</p> : state === "loading" ? <p className="notice" role="status">Loading the latest record…</p> : undefined;
  return <StudentPairScreen code={code} state={state} notice={notice} onCodeChange={setCode} onSubmit={submit} />;
}

export const Pairing: Story = {
  render: () => <PairingStory />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.type(canvas.getByLabelText("Pairing code"), "PRIMER-4821");
    await expect(canvas.getByRole("button", { name: "Pair this browser" })).toBeEnabled();
  },
};

export const PairingExpired: Story = { render: () => <PairingStory initialState="expired" /> };
const storyLink = (item: ChecklistScreenItem, children: ReactNode, className: string) => <a className={className} href={item.href} aria-label={`Open ${item.title}, ${item.status}`} key={item.id}>{children}</a>;
export const EmptyChecklist: Story = { render: () => <StudentChecklistScreen name="Ada" state="empty" items={[]} onRefresh={noop} /> };
export const MixedChecklist: Story = {
  render: () => <StudentChecklistScreen name="Ada" state="ready" items={checklistItems} renderLink={storyLink} />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const visibleTasks = canvas.getAllByRole("link");
    await expect(visibleTasks[0]).toHaveAccessibleName(/Revise history essay, In progress/);
    await expect(visibleTasks[1]).toHaveAccessibleName(/Measure the workbench, Needs another try/);
    await expect(visibleTasks[2]).toHaveAccessibleName(/Practice fractions, Ready to start/);
    await expect(canvas.queryByText("Approved", { exact: true })).not.toBeInTheDocument();
    await userEvent.click(canvas.getByRole("button", { name: "Show completed tasks (3)" }));
    await expect(canvas.getByText("Approved", { exact: true })).toBeVisible();
    await expect(canvas.getByText("Skipped", { exact: true })).toBeVisible();
    await expect(canvas.getByText("Canceled", { exact: true })).toBeVisible();
    await userEvent.click(canvas.getByRole("button", { name: "Hide completed tasks" }));
  },
};
const notStarted: StudentOccurrenceScreenProps = { title: "Practice fractions", instructions: checklistItems[0].description, status: "Ready to start", action: "start", onAction: noop, onRefresh: noop, onBack: noop };
export const NotStarted: Story = { render: () => occurrence(notStarted) };
export const InProgress: Story = { render: () => occurrence({ ...notStarted, status: "In progress", action: "submit" }) };
export const WaitingForParent: Story = { render: () => occurrence({ ...notStarted, status: "Sent to your parent", action: undefined, explanation: "Your work is saved. You can return to today’s tasks while your parent checks it." }) };
export const RejectedRetry: Story = { render: () => occurrence({ ...notStarted, status: "Needs another try", action: "retry", explanation: "Try the task again with care. Your previous work is still saved." }) };
export const Completed: Story = { render: () => occurrence({ ...notStarted, status: "Approved", action: undefined, explanation: "Nice work. Your parent approved this task." }) };
export const Unsupported: Story = { render: () => occurrence({ ...notStarted, status: "Unavailable on this device", action: undefined, supported: false, explanation: "Ask your parent for help opening this task on a supported device." }) };
