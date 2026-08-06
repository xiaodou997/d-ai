<script setup lang="ts">
import { computed } from "vue";

import PortalContentCard from "../../../page/PortalContentCard.vue";
import {
  appCurlExample,
  appInputFields,
  appOutputFields,
  appRunPath
} from "../../apps/contract";
import type { PortalAppType } from "../../apps/types";
import type { PortalAiDocsScope } from "../../docs";
import { portalAiDocsAppLabel } from "../../docs";
import PortalAiDocsCodeBlock from "../PortalAiDocsCodeBlock.vue";
import PortalAiDocsTabbedCode from "../PortalAiDocsTabbedCode.vue";

const props = defineProps<{
  baseUrl: string;
  scope: PortalAiDocsScope;
}>();

const appLabel = computed(() => portalAiDocsAppLabel(props.scope));

const chatVariables = ["brand", "tone"];
const imageVariables = ["scene", "style"];
const variableTemplateExample = "你是{{brand}}的顾问，用{{tone}}语气回答。";
const variablePayloadExample = `{
  "input": "请介绍一下年度版套餐",
  "variables": {
    "brand": "Acme",
    "tone": "正式"
  }
}`;

const chatInputFields = computed(() => appInputFields("chat", chatVariables, true));
const imageGenerationFields = computed(() => appInputFields("image_generation", imageVariables));
const imageEditFields = computed(() => appInputFields("image_edit", imageVariables));
const chatOutputFields = computed(() => appOutputFields("chat"));
const imageOutputFields = computed(() => appOutputFields("image_generation"));

const attachmentItemFields = [
  { name: "type", required: "否", desc: "image 或 file。缺省时按 mime_type 或 URL 后缀推断图片。" },
  { name: "url", required: "是", desc: "直连 http(s) 地址；运行时会拉取这个资源。" },
  { name: "name", required: "否", desc: "文件名，便于 file 类型展示和透传。" },
  { name: "mime_type", required: "否", desc: "补充 MIME 信息，帮助把资源识别成图片或文件。" }
] as const;

const imageRuntimeRules = [
  { subject: "调用方可传", desc: "input、variables、images（图生图）、response_format、stream。" },
  { subject: "应用固定", desc: "只有分辨率是应用配置项，调用方不能覆盖。" },
  { subject: "不对调用方开放", desc: "size、background、output_format 会触发应用参数锁定；quality、style 会被忽略。" },
  { subject: "其余参数", desc: "走服务端默认值，调用方不需要理解底层差异。" }
] as const;

const endpointCards: Array<{ appType: PortalAppType; title: string; note: string }> = [
  {
    appType: "chat",
    title: "对话应用",
    note: "调用方传 input / variables / attachments / stream；temperature、max_tokens 这类运行参数由应用锁定。"
  },
  {
    appType: "image_generation",
    title: "文生图应用",
    note: "调用方传 input / variables / response_format / stream；分辨率由应用固定，其它图像细项不对调用方开放。"
  },
  {
    appType: "image_edit",
    title: "图生图应用",
    note: "在文生图基础上额外传 images 作为参考图；size、background、output_format 由应用锁定，quality、style 会被忽略。"
  }
];

function runPath(appType: PortalAppType) {
  return appRunPath(appType);
}

function runKeyPython(path: string, payload: string) {
  return `import requests

resp = requests.post(
    "${props.baseUrl}${path}",
    headers={
        "Authorization": "Bearer rk_xxx",
        "Content-Type": "application/json",
    },
    json=${payload},
)

print(resp.json())`;
}

function runKeyNode(path: string, body: string) {
  return `const resp = await fetch("${props.baseUrl}${path}", {
  method: "POST",
  headers: {
    "Authorization": "Bearer rk_xxx",
    "Content-Type": "application/json"
  },
  body: JSON.stringify(${body})
});

console.log(await resp.json());`;
}

const chatRunExamples = computed(() => {
  const payload = `{
  "input": "请根据模板输出一段正式的售前回复",
  "variables": {
    "brand": "Acme",
    "tone": "正式"
  },
  "attachments": [
    {"type": "image", "url": "https://example.com/photo.png", "name": "photo.png", "mime_type": "image/png"}
  ],
  "stream": false
}`;
  return [
    {
      key: "curl",
      label: "cURL",
      code: appCurlExample("chat", chatVariables, "caller_variables", props.baseUrl, "rk_xxx", true)
    },
    {
      key: "python",
      label: "Python",
      code: runKeyPython(runPath("chat"), payload)
    },
    {
      key: "node",
      label: "Node",
      code: runKeyNode(runPath("chat"), payload)
    }
  ];
});

const imageRunExamples = computed(() => {
  const payload = `{
  "input": "保留产品主体，强化高级感棚拍布光",
  "variables": {
    "scene": "新品发布会主视觉",
    "style": "极简"
  },
  "response_format": "b64_json",
  "stream": false
}`;
  return [
    {
      key: "curl",
      label: "cURL",
      code: appCurlExample("image_generation", imageVariables, "caller_variables", props.baseUrl, "rk_xxx")
    },
    {
      key: "python",
      label: "Python",
      code: runKeyPython(runPath("image_generation"), payload)
    },
    {
      key: "node",
      label: "Node",
      code: runKeyNode(runPath("image_generation"), payload)
    }
  ];
});

const imageEditCurl = computed(() => appCurlExample("image_edit", imageVariables, "caller_variables", props.baseUrl, "rk_xxx"));
</script>

<template>
  <div class="ai-docs-stack">
    <PortalContentCard title="应用与应用密钥" description="应用密钥用于调用已经配置好的应用，而不是直接操作底层模型。">
      <div class="ai-docs-grid">
        <article class="ai-docs-note">
          <strong class="ai-docs-note__head">{{ appLabel }}</strong>
          <div class="ai-docs-note__body">应用 = 提示词模板 + 绑定模型 + 运行配置。调用方只需传 input 和变量，不需要接触模板原文和底层参数。</div>
        </article>
        <article class="ai-docs-note">
          <strong class="ai-docs-note__head">应用密钥</strong>
          <div class="ai-docs-note__body">
            <span class="ai-docs-inline-code">rk_</span> 只能绑定一个应用，只能打
            <span class="ai-docs-inline-code">POST /v1/run</span> 这一个入口；应在“应用密钥”页创建，并绑定到某个应用。
          </div>
        </article>
      </div>
      <p class="ai-docs-lead">
        创建应用的步骤在「应用管理」页，这里只说明怎么调用。创建完应用后，去「应用密钥」页生成
        <span class="ai-docs-inline-code">rk_</span> 分发给调用方；密钥支持随时查看明文和一键轮换。
      </p>
      <div class="ai-docs-callout ai-docs-callout--info">
        <strong class="ai-docs-callout__title">统一入口，一个密钥打所有应用类型</strong>
        <p class="ai-docs-callout__body">
          对话、文生图、图生图三类应用统一通过
          <span class="ai-docs-inline-code">{{ baseUrl }}/v1/run</span>
          调用——密钥绑定的应用类型决定了具体行为，调用方不需要区分不同服务路径。
        </p>
      </div>
    </PortalContentCard>

    <div class="ai-docs-grid">
      <PortalContentCard title="input vs variables" description="这两个字段最容易混淆，务必分清。">
        <table class="ai-docs-table">
          <thead>
            <tr><th>字段</th><th>它表示什么</th></tr>
          </thead>
          <tbody>
            <tr>
              <td><code>input</code></td>
              <td>本次用户输入；对话、文生图和图生图统一使用该字段。</td>
            </tr>
            <tr>
              <td><code>variables</code></td>
              <td>给应用模板里的 <code v-pre>{{ placeholder }}</code> 填值，类似“填空”——变量名在应用详情的「输入变量」里查看。</td>
            </tr>
            <tr>
              <td><code>input</code> 中的 <code v-pre>{{提示词名称}}</code></td>
              <td>仅动态提示词组合使用；按当前应用已绑定的提示词名称精确匹配并替换，名称可以是中文。</td>
            </tr>
          </tbody>
        </table>
        <div class="ai-docs-note">
          <strong class="ai-docs-note__head">模板替换规则</strong>
          <div class="ai-docs-note__body">
            占位符必须使用英文半角 <span class="ai-docs-inline-code" v-pre>{{ }}</span>，名称可以是中文。
            多余变量会被忽略；提示词变量漏传或动态提示词名称未绑定时，请求会返回明确错误。
          </div>
        </div>
      </PortalContentCard>

      <PortalContentCard title="变量示例" description="调用方只知道变量名，看不到模板原文。">
        <PortalAiDocsCodeBlock
          title="模板示例（用于理解，不会在调用方界面明文展示）"
          :code="variableTemplateExample"
        />
        <PortalAiDocsCodeBlock
          title="调用方传参"
          :code="variablePayloadExample"
        />
      </PortalContentCard>
    </div>

    <PortalContentCard title="三类应用" description="统一走同一个 POST /v1/run 入口；被应用锁定的参数不允许再覆盖，调用方无需关心路径差异。">
      <div class="ai-docs-grid ai-docs-grid--three">
        <article v-for="item in endpointCards" :key="item.title" class="ai-docs-note">
          <strong class="ai-docs-note__head">{{ item.title }}</strong>
          <div class="ai-docs-note__chips">
            <span class="ai-docs-badge" :class="item.appType === 'chat' ? 'ai-docs-badge--chat' : 'ai-docs-badge--image'">{{ item.appType }}</span>
            <span class="ai-docs-inline-code">POST {{ runPath(item.appType) }}</span>
          </div>
          <div class="ai-docs-note__body">{{ item.note }}</div>
        </article>
      </div>
    </PortalContentCard>

    <div class="ai-docs-grid">
      <PortalContentCard title="对话应用 · 输入字段" description="对话应用入口是 POST /v1/run；attachments 是对象数组，不是字符串数组。">
        <div class="ai-docs-section-head">
          <div class="ai-docs-chip-row">
            <span class="ai-docs-badge ai-docs-badge--chat">chat</span>
            <span class="ai-docs-badge ai-docs-badge--runkey">rk_</span>
          </div>
          <p class="ai-docs-lead">调用方只传一次性 input、模板变量和可选附件；不直接控制底层模型运行参数。</p>
        </div>
        <table class="ai-docs-table">
          <thead>
            <tr><th>输入字段</th><th>必填</th><th>说明</th></tr>
          </thead>
          <tbody>
            <tr v-for="field in chatInputFields" :key="field.name">
              <td><code>{{ field.name }}</code></td>
              <td>{{ field.required }}</td>
              <td>{{ field.desc }}</td>
            </tr>
          </tbody>
        </table>
      </PortalContentCard>

      <PortalContentCard title="对话应用 · 输出与附件对象项" description="把返回字段和 attachments 子结构单独列出来，避免和其他应用类型混在一起。">
        <div class="ai-docs-section-stack">
          <div class="ai-docs-table-caption">返回字段</div>
          <table class="ai-docs-table">
            <thead>
              <tr><th>输出字段</th><th>说明</th></tr>
            </thead>
            <tbody>
              <tr v-for="field in chatOutputFields" :key="field.name">
                <td><code>{{ field.name }}</code></td>
                <td>{{ field.desc }}</td>
              </tr>
            </tbody>
          </table>

          <div class="ai-docs-table-caption">attachments 对象项</div>
          <table class="ai-docs-table">
            <thead>
              <tr><th>字段</th><th>必填</th><th>说明</th></tr>
            </thead>
            <tbody>
              <tr v-for="field in attachmentItemFields" :key="field.name">
                <td><code>{{ field.name }}</code></td>
                <td>{{ field.required }}</td>
                <td>{{ field.desc }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </PortalContentCard>
    </div>

    <div class="ai-docs-grid">
      <PortalContentCard title="文生图应用 · 输入字段" description="入口同样是 POST /v1/run；这里只保留业务调用方真正需要知道的字段。">
        <div class="ai-docs-section-head">
          <div class="ai-docs-chip-row">
            <span class="ai-docs-badge ai-docs-badge--image">image_generation</span>
            <span class="ai-docs-badge ai-docs-badge--runkey">rk_</span>
          </div>
          <p class="ai-docs-lead">调用方可以补充 input 和 variables，并选择返回 URL 还是 Base64；不直接碰底层图片调参。</p>
        </div>
        <table class="ai-docs-table">
          <thead>
            <tr><th>字段</th><th>必填</th><th>说明</th></tr>
          </thead>
          <tbody>
            <tr v-for="field in imageGenerationFields" :key="field.name">
              <td><code>{{ field.name }}</code></td>
              <td>{{ field.required }}</td>
              <td>{{ field.desc }}</td>
            </tr>
          </tbody>
        </table>
        <div class="ai-docs-callout ai-docs-callout--warning">
          <strong class="ai-docs-callout__title">不要把原生生图字段照搬到应用密钥</strong>
          <p class="ai-docs-callout__body">
            对应用密钥来说，调用方唯一可选的图片返回相关字段是 <code>response_format</code>。
            <code>size</code>、<code>background</code>、<code>output_format</code> 不对调用方开放；
            <code>quality</code>、<code>style</code> 传入时会被忽略。
          </p>
        </div>
      </PortalContentCard>

      <PortalContentCard title="图生图应用 · 输入字段" description="入口同样是 POST /v1/run；在文生图基础上增加参考图。">
        <div class="ai-docs-section-head">
          <div class="ai-docs-chip-row">
            <span class="ai-docs-badge ai-docs-badge--image">image_edit</span>
            <span class="ai-docs-badge ai-docs-badge--runkey">rk_</span>
          </div>
          <p class="ai-docs-lead">除了 input / variables / response_format / stream，还必须提供至少一张参考图。</p>
        </div>
        <table class="ai-docs-table">
          <thead>
            <tr><th>字段</th><th>必填</th><th>说明</th></tr>
          </thead>
          <tbody>
            <tr v-for="field in imageEditFields" :key="field.name">
              <td><code>{{ field.name }}</code></td>
              <td>{{ field.required }}</td>
              <td>{{ field.desc }}</td>
            </tr>
          </tbody>
        </table>
        <div class="ai-docs-callout ai-docs-callout--info">
          <strong class="ai-docs-callout__title">images 参数格式</strong>
          <p class="ai-docs-callout__body">
            必须传对象数组，每项形如 <code>{ "image_url": "..." }</code>。<code>image_url</code>
            可以是 HTTP(S) 直连 URL，也可以是 <code>data:image/...;base64,...</code>；图片来源对象当前只映射 <code>image_url</code>。
          </p>
        </div>
      </PortalContentCard>
    </div>

    <PortalContentCard title="统一返回与运行规则" description="把应用密钥的返回格式和限制单独拿出来说明，避免和直接调模型的接口混淆。">
      <div class="ai-docs-grid">
        <div class="ai-docs-section-stack">
          <div class="ai-docs-table-caption">统一图片返回字段</div>
          <table class="ai-docs-table">
            <thead>
              <tr><th>字段</th><th>说明</th></tr>
            </thead>
            <tbody>
              <tr v-for="field in imageOutputFields" :key="field.name">
                <td><code>{{ field.name }}</code></td>
                <td>{{ field.desc }}</td>
              </tr>
            </tbody>
          </table>
        </div>
        <div class="ai-docs-section-stack">
          <div class="ai-docs-table-caption">应用密钥规则</div>
          <table class="ai-docs-table">
            <thead>
              <tr><th>主题</th><th>说明</th></tr>
            </thead>
            <tbody>
              <tr v-for="rule in imageRuntimeRules" :key="rule.subject">
                <td>{{ rule.subject }}</td>
                <td>{{ rule.desc }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
      <div class="ai-docs-callout ai-docs-callout--warning">
        <strong class="ai-docs-callout__title">锁定参数与错误返回</strong>
        <p class="ai-docs-callout__body">
          对话应用如果传 <code>temperature</code>、<code>max_tokens</code>，以及图片应用如果传
          <code>size</code>、<code>background</code>、<code>output_format</code>，会返回 <code>400 invalid_request_error</code>；
          <code>quality</code>、<code>style</code> 会被忽略。附件未开启、附件超限、附件 URL 不是直连 http(s) 地址，也会走同类 400。
        </p>
      </div>
    </PortalContentCard>

    <PortalContentCard title="多语言示例" description="以下是三类应用的调用示例，全部走同一个 POST /v1/run 入口。">
      <PortalAiDocsTabbedCode title="对话应用 · POST /v1/run" :tabs="chatRunExamples" />
      <PortalAiDocsTabbedCode title="文生图应用 · POST /v1/run" :tabs="imageRunExamples" />
      <PortalAiDocsCodeBlock
        title="图生图应用 · cURL（POST /v1/run）"
        :code="imageEditCurl"
        caption="图生图额外要求 images 对象数组；每项 image_url 可使用 HTTP(S) URL 或 base64 data URL。"
      />
    </PortalContentCard>
  </div>
</template>
