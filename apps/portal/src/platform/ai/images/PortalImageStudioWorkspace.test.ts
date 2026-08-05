import { flushPromises, shallowMount } from "@vue/test-utils";
import { afterEach, describe, expect, it, vi } from "vitest";

import PortalImageStudioWorkspace from "./PortalImageStudioWorkspace.vue";
import PortalImageReferenceTray from "./PortalImageReferenceTray.vue";

import type { PortalImageApi, PortalImageJobRecord } from "./types";

const ElInputStub = {
  name: "ElInput",
  props: ["modelValue"],
  emits: ["update:modelValue"],
  template: "<textarea :value=\"modelValue\" />"
};

afterEach(() => {
  vi.useRealTimers();
  vi.restoreAllMocks();
});

describe("PortalImageStudioWorkspace", () => {
  it("uses the configured polling interval for active tasks", async () => {
    vi.useFakeTimers();
    const activeJob: PortalImageJobRecord = {
      id: "task-1",
      model_code: "gpt-image-1",
      prompt: "lighthouse",
      status: "running",
      storage_policy: "temporary",
      raw_image_retained: false,
      caller_charge_credits: 0,
      image_count: 0,
      inline_count: 0,
      url_count: 0,
      created_at: Date.now()
    };
    const api: PortalImageApi = {
      listModels: vi.fn().mockResolvedValue([]),
      listJobs: vi.fn().mockResolvedValue([activeJob]),
      createTask: vi.fn(),
      getTask: vi.fn().mockResolvedValue({ ...activeJob, status: "completed" }),
      cancelTask: vi.fn(),
      deleteTask: vi.fn()
    };

    const wrapper = shallowMount(PortalImageStudioWorkspace, {
      props: {
        api,
        formatCredits: (value) => String(value ?? 0),
        usageMessage: "usage",
        pollIntervalMs: 1_234
      },
      global: { config: { warnHandler: () => undefined } }
    });
    await flushPromises();

    await vi.advanceTimersByTimeAsync(1_233);
    expect(api.getTask).not.toHaveBeenCalled();
    await vi.advanceTimersByTimeAsync(1);
    await flushPromises();
    expect(api.getTask).toHaveBeenCalledOnce();
    expect(api.getTask).toHaveBeenCalledWith("task-1");

    wrapper.unmount();
  });

  it("shows delete for terminal tasks and removes the record after confirmation", async () => {
    const completedJob: PortalImageJobRecord = {
      id: "task-completed",
      model_code: "gpt-image-1",
      prompt: "sunset over the ocean",
      status: "completed",
      storage_policy: "temporary",
      raw_image_retained: true,
      caller_charge_credits: 5,
      image_count: 1,
      inline_count: 0,
      url_count: 0,
      created_at: Date.now()
    };
    const confirmDelete = vi.fn().mockResolvedValue(true);
    const api: PortalImageApi = {
      listModels: vi.fn().mockResolvedValue([]),
      listJobs: vi.fn().mockResolvedValue([completedJob]),
      createTask: vi.fn(),
      getTask: vi.fn(),
      cancelTask: vi.fn(),
      deleteTask: vi.fn().mockResolvedValue({ deleted: true })
    };

    const wrapper = shallowMount(PortalImageStudioWorkspace, {
      props: {
        api,
        formatCredits: (value) => String(value ?? 0),
        usageMessage: "usage",
        confirmDelete
      },
      global: { config: { warnHandler: () => undefined } }
    });
    await flushPromises();

    expect(wrapper.find('[aria-label="取消任务"]').exists()).toBe(false);
    const deleteButton = wrapper.find('[aria-label="删除任务记录"]');
    expect(deleteButton.exists()).toBe(true);
    await deleteButton.trigger("click");
    await flushPromises();

    expect(confirmDelete).toHaveBeenCalledWith(completedJob);
    expect(api.deleteTask).toHaveBeenCalledWith(completedJob.id);
    expect(wrapper.find('[aria-label="删除任务记录"]').exists()).toBe(false);

    wrapper.unmount();
  });

  it("submits reference images in visual order and keeps prompt references aligned", async () => {
    vi.spyOn(URL, "createObjectURL").mockImplementation((file) => `blob:${(file as File).name}`);
    vi.spyOn(URL, "revokeObjectURL").mockImplementation(() => undefined);
    const createdJob: PortalImageJobRecord = {
      id: "task-new",
      operation: "edit",
      model_code: "image-model",
      prompt: "use references",
      status: "pending",
      storage_policy: "temporary",
      raw_image_retained: false,
      caller_charge_credits: 0,
      image_count: 0,
      inline_count: 0,
      url_count: 0,
      created_at: Date.now()
    };
    const api: PortalImageApi = {
      listModels: vi.fn().mockResolvedValue([
        { model_code: "image-model", capability_type: "image", status: "active" }
      ]),
      listJobs: vi.fn().mockResolvedValue([]),
      createTask: vi.fn().mockResolvedValue({ task_id: createdJob.id, status: "pending" }),
      getTask: vi.fn().mockResolvedValue(createdJob),
      cancelTask: vi.fn(),
      deleteTask: vi.fn()
    };
    const wrapper = shallowMount(PortalImageStudioWorkspace, {
      props: {
        api,
        formatCredits: (value) => String(value ?? 0),
        usageMessage: "usage"
      },
      global: {
        config: { warnHandler: () => undefined },
        stubs: { ElInput: ElInputStub }
      }
    });
    await flushPromises();

    const first = new File(["first"], "first.png", { type: "image/png" });
    const second = new File(["second"], "second.png", { type: "image/png" });
    const tray = wrapper.findComponent(PortalImageReferenceTray);
    tray.vm.$emit("add", [first, second]);
    await flushPromises();

    const promptInput = wrapper.findAllComponents({ name: "ElInput" })[0];
    expect(promptInput).toBeDefined();
    promptInput.vm.$emit("update:modelValue", "use @图片1 layout and @图片2 subject");
    await flushPromises();
    const references = tray.props("references");
    tray.vm.$emit("move", { id: references[1].id, direction: -1 });
    await flushPromises();

    await wrapper.find(".composer-send").trigger("click");
    await flushPromises();

    expect(api.createTask).toHaveBeenCalledOnce();
    const payload = vi.mocked(api.createTask).mock.calls[0][0];
    expect(payload).toBeInstanceOf(FormData);
    const form = payload as FormData;
    expect(form.get("operation")).toBe("edit");
    expect(form.getAll("image[]")).toEqual([second, first]);
    expect(form.get("prompt")).toBe("use @图片2 layout and @图片1 subject");
    expect(form.get("n")).toBe("1");
    expect(form.get("size")).toBe("auto");

    wrapper.unmount();
  });
});
