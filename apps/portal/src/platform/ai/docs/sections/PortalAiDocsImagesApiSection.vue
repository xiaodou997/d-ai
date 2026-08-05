<script setup lang="ts">
import { computed } from "vue";

import PortalContentCard from "../../../page/PortalContentCard.vue";
import PortalAiDocsTabbedCode from "../PortalAiDocsTabbedCode.vue";

const props = defineProps<{
  baseUrl: string;
}>();

const imageRuleCards = [
  {
    title: "最稳的最小集合",
    badge: "推荐",
    desc: "先只传 model、prompt、size、response_format。大多数接入场景先靠这四个字段就够。"
  },
  {
    title: "平台支持的可选项",
    badge: "按模型支持",
    desc: "background、output_format 等当前 Schema 字段按 GPT Image 契约转发；quality、style 会被忽略。"
  },
  {
    title: "不要混淆两类格式",
    badge: "重要",
    desc: "response_format 决定返回里拿 URL 还是 Base64；output_format 决定图片文件的编码格式（如 png/jpeg）。"
  }
];

const stableFields = [
  { name: "model", level: "必需", desc: "平台模型列表里的生图模型编码。" },
  { name: "prompt", level: "必需", desc: "本次图像生成提示词。" },
  { name: "n", level: "可选", desc: "输出图片张数，1–10；还需满足所选上游模型绑定的最大张数。" },
  { name: "size", level: "推荐", desc: "输出分辨率，例如 1024x1024、1536x1024。" },
  { name: "response_format", level: "推荐", desc: "返回包里拿 url 还是 b64_json；默认通常用 b64_json。" }
];

const optionalPassthroughFields = [
  { name: "background", desc: "背景处理策略，如透明 / 实底 / auto；具体取值看模型支持。" },
  { name: "output_format", desc: "图片文件编码格式，如 png、jpeg、webp；和 response_format 不是一回事。" }
];

const imageEditFields = [
  { name: "model", json: "是", multipart: "是", desc: "平台生图模型编码。" },
  { name: "images", json: "是", multipart: "—", desc: "对象数组，每项只包含 image_url；支持 HTTP(S) URL 或 data URL。" },
  { name: "image[]", json: "—", multipart: "是", desc: "重复的文件字段，每个字段上传一张参考图。" },
  { name: "mask", json: "对象", multipart: "文件", desc: "可选蒙版；JSON 使用 { image_url }，multipart 上传文件。" },
  { name: "prompt", json: "是", multipart: "是", desc: "图片编辑提示词。" },
  { name: "n", json: "可选", multipart: "可选", desc: "输出图片张数，1–10；和最多 16 张参考输入图是两个不同概念。" },
  { name: "size / background", json: "可选", multipart: "可选", desc: "输出图片参数。" },
  { name: "input_fidelity / moderation", json: "可选", multipart: "可选", desc: "输入保真度与内容审核配置。" },
  { name: "output_format / output_compression", json: "可选", multipart: "可选", desc: "输出编码和压缩率。" },
  { name: "response_format", json: "可选", multipart: "可选", desc: "仅控制平台返回 url 或 b64_json，不发送给 GPT Image 上游。" }
] as const;

const imageExamples = computed(() => [
  {
    key: "curl",
    label: "cURL",
    code: `curl ${props.baseUrl}/v1/images/generations \\
  -H "Content-Type: application/json" \\
  -H "Authorization: Bearer sk_xxx" \\
  -d '{
    "model": "gpt-image-1",
    "prompt": "为企业管理后台生成一张低饱和、干净的首屏插图",
    "n": 2,
    "size": "1536x1024",
    "response_format": "b64_json"
  }'`
  },
  {
    key: "python",
    label: "Python",
    code: `from openai import OpenAI

client = OpenAI(
    api_key="sk_xxx",
    base_url="${props.baseUrl}",
)

resp = client.images.generate(
    model="gpt-image-1",
    prompt="为企业管理后台生成一张低饱和、干净的首屏插图",
    n=2,
    size="1536x1024",
    response_format="b64_json",
)

print(resp.data[0].b64_json)`
  },
  {
    key: "node",
    label: "Node",
    code: `import OpenAI from "openai";

const client = new OpenAI({
  apiKey: "sk_xxx",
  baseURL: "${props.baseUrl}",
});

const resp = await client.images.generate({
  model: "gpt-image-1",
  prompt: "为企业管理后台生成一张低饱和、干净的首屏插图",
  n: 2,
  size: "1536x1024",
  response_format: "b64_json",
});

console.log(resp.data[0].b64_json);`
  }
]);

const imageEditExamples = computed(() => [
  {
    key: "json",
    label: "JSON",
    code: `curl ${props.baseUrl}/v1/images/edits \\
  -H "Content-Type: application/json" \\
  -H "Authorization: Bearer sk_xxx" \\
  -d '{
    "model": "gpt-image-1",
    "images": [
      {"image_url": "https://example.com/reference.png"}
    ],
    "prompt": "保留主体构图，优化棚拍光线",
    "input_fidelity": "high",
    "response_format": "url"
  }'`
  },
  {
    key: "multipart",
    label: "Multipart",
    code: `curl ${props.baseUrl}/v1/images/edits \\
  -H "Authorization: Bearer sk_xxx" \\
  -F "model=gpt-image-1" \\
  -F "image[]=@reference.png" \\
  -F "prompt=保留主体构图，优化棚拍光线" \\
  -F "input_fidelity=high" \\
  -F "response_format=url"`
  }
]);
</script>

<template>
  <div class="ai-docs-stack">
    <PortalContentCard title="接口定位" description="这是直接调用模型的生图接口，使用 API 密钥（sk_）。如果你要用配置好的应用生成图片，请看运行密钥文档。">
      <div class="ai-docs-endpoint-list">
        <article class="ai-docs-endpoint">
          <span class="ai-docs-endpoint__method">POST</span>
          <div>
            <div class="ai-docs-endpoint__path">/v1/images/generations</div>
            <p class="ai-docs-endpoint__desc">文生图入口。平台只做鉴权和转发，直接把参数传给模型处理。</p>
          </div>
        </article>
        <article class="ai-docs-endpoint">
          <span class="ai-docs-endpoint__method">POST</span>
          <div>
            <div class="ai-docs-endpoint__path">/v1/images/edits</div>
            <p class="ai-docs-endpoint__desc">图生图入口。JSON 与 multipart 是同一参数模型的两种 HTTP 编码。</p>
          </div>
        </article>
      </div>
      <div class="ai-docs-grid ai-docs-grid--three">
        <article v-for="item in imageRuleCards" :key="item.title" class="ai-docs-note">
          <div class="ai-docs-note__chips">
            <span class="ai-docs-badge ai-docs-badge--accent">{{ item.badge }}</span>
          </div>
          <strong class="ai-docs-note__head">{{ item.title }}</strong>
          <div class="ai-docs-note__body">{{ item.desc }}</div>
        </article>
      </div>
    </PortalContentCard>

    <div class="ai-docs-grid">
      <PortalContentCard title="请求字段" description="接口参数和 OpenAI Images 接口一致，但不是每个模型都支持所有字段。">
        <div class="ai-docs-section-stack">
          <div class="ai-docs-table-caption">稳定字段</div>
          <table class="ai-docs-table">
            <thead>
              <tr><th>字段</th><th>建议</th><th>说明</th></tr>
            </thead>
            <tbody>
              <tr v-for="field in stableFields" :key="field.name">
                <td><code>{{ field.name }}</code></td>
                <td>{{ field.level }}</td>
                <td>{{ field.desc }}</td>
              </tr>
            </tbody>
          </table>

          <div class="ai-docs-table-caption">模型相关可选字段</div>
          <table class="ai-docs-table">
            <thead>
              <tr><th>字段</th><th>说明</th></tr>
            </thead>
            <tbody>
              <tr v-for="field in optionalPassthroughFields" :key="field.name">
                <td><code>{{ field.name }}</code></td>
                <td>{{ field.desc }}</td>
              </tr>
            </tbody>
          </table>
        </div>
        <div class="ai-docs-callout ai-docs-callout--warning">
          <strong class="ai-docs-callout__title">模型相关字段按需使用</strong>
          <p class="ai-docs-callout__body">
            <code>background</code>、<code>output_format</code> 是否生效取决于所选模型。
            <code>quality</code>、<code>style</code> 不属于平台输入 Schema，传入时会被忽略。
          </p>
        </div>
      </PortalContentCard>

      <PortalContentCard title="返回说明" description="返回格式和 OpenAI Images 接口一致。response_format 决定你拿 URL 还是 Base64。">
        <ul class="ai-docs-list">
          <li><code>response_format = url</code> → 遍历 <code>data[].url</code> 取图片地址。</li>
          <li><code>response_format = b64_json</code> → 遍历 <code>data[].b64_json</code> 取 Base64 数据。</li>
          <li><code>output_format</code> 决定图片文件是 png / jpeg / webp 哪种编码，取决于模型是否支持。</li>
          <li>是否返回修订后的 prompt 取决于模型响应，平台不额外干预。</li>
        </ul>
        <div class="ai-docs-note">
          <strong class="ai-docs-note__head">想固定分辨率和模型？</strong>
          <div class="ai-docs-note__body">把生图能力做成应用，然后给调用方发 <span class="ai-docs-inline-code">rk_</span> 运行密钥，调用方就不用关心这些参数了。</div>
        </div>
      </PortalContentCard>
    </div>

    <PortalContentCard title="图生图请求" description="只映射当前 OpenAI Images Edit Schema；未列出的 JSON 属性和 multipart 普通字段会被忽略。">
      <table class="ai-docs-table">
        <thead>
          <tr><th>逻辑字段</th><th>JSON</th><th>Multipart</th><th>说明</th></tr>
        </thead>
        <tbody>
          <tr v-for="field in imageEditFields" :key="field.name">
            <td><code>{{ field.name }}</code></td>
            <td>{{ field.json }}</td>
            <td>{{ field.multipart }}</td>
            <td>{{ field.desc }}</td>
          </tr>
        </tbody>
      </table>
      <div class="ai-docs-callout ai-docs-callout--warning">
        <strong class="ai-docs-callout__title">输出数量取决于模型绑定</strong>
        <p class="ai-docs-callout__body"><code>n</code> 支持 1–10，但不能超过所选上游模型绑定的生成/编辑最大张数；<code>partial_images</code> 不受支持；<code>quality</code>、<code>style</code> 会被忽略。</p>
      </div>
    </PortalContentCard>

    <PortalContentCard title="多语言示例" description="以下示例都走 API 密钥（sk_），适合服务端脚本和内部工具直接接入。">
      <PortalAiDocsTabbedCode title="POST /v1/images/generations" :tabs="imageExamples" />
      <PortalAiDocsTabbedCode title="POST /v1/images/edits" :tabs="imageEditExamples" />
    </PortalContentCard>
  </div>
</template>
