export type LegalDocumentID =
  "privacy" | "terms" | "cookies" | "acceptable-use";

export interface LegalDocumentSection {
  heading: string;
  paragraphs: string[];
  items?: string[];
}

export interface LegalDocument {
  id: LegalDocumentID;
  title: string;
  summary: string;
  version: string;
  effectiveDate: string;
  sections: LegalDocumentSection[];
}

const DOCUMENT_VERSION = "2026-07-19";

export const legalDocuments: readonly LegalDocument[] = [
  {
    id: "privacy",
    title: "隐私政策",
    summary: "说明平台处理个人信息的目的、范围、保存方式与联系渠道。",
    version: DOCUMENT_VERSION,
    effectiveDate: "2026-07-19",
    sections: [
      {
        heading: "适用范围",
        paragraphs: [
          "本政策适用于豆栈 DouStack 平台、其管理端、租户端、用户端及相关身份认证和开发者服务。租户邀请终端用户使用服务时，租户与平台各自承担的数据处理角色应以适用合同和实际业务关系为准。",
          "在平台运营主体、联系方式、适用地区或数据处理安排尚未在部署配置中确认前，本政策不得替代正式的法务审阅文本。",
        ],
      },
      {
        heading: "我们处理的信息",
        paragraphs: [
          "我们仅为提供、保护、运维和改进服务所必需的目的处理信息。",
        ],
        items: [
          "账户与组织信息，例如用户名、邮箱、手机号、租户标识、角色和登录状态。",
          "服务使用信息，例如认证审计、充值或账务记录、调用量、错误记录和安全事件。",
          "您或代表您提交的内容，例如 AI 请求、附件、提示词和生成结果；请勿在没有合法权限的情况下提交敏感或受限制信息。",
        ],
      },
      {
        heading: "AI 服务与第三方处理",
        paragraphs: [
          "为完成您发起的 AI 请求，平台可能将请求内容、必要的技术元数据和附件发送至您选择或平台配置的上游服务商。请在使用前确认您有权进行该等传输，并结合所选模型和上游服务商的条款评估数据处理安排。",
        ],
      },
      {
        heading: "保存、安全与您的请求",
        paragraphs: [
          "不同数据类别的保存期限由实际运行配置、账务与审计需要以及适用法律决定。平台采取访问控制、传输安全和日志脱敏等措施，但不存在绝对安全的网络环境。",
          "如需查询、更正、删除、导出或投诉，请通过平台公布的联系渠道提出请求。平台会在核验身份及评估租户、账务、审计和法定义务后处理。",
        ],
      },
    ],
  },
  {
    id: "terms",
    title: "服务条款",
    summary: "规定账户使用、服务边界、费用、内容责任和条款变更规则。",
    version: DOCUMENT_VERSION,
    effectiveDate: "2026-07-19",
    sections: [
      {
        heading: "账户与授权",
        paragraphs: [
          "您应提供真实、准确且有权提供的账户信息，并妥善保管密码、访问令牌和 API 密钥。通过租户邀请注册的用户，应同时遵守所属租户对该账户和业务数据的管理规则。",
        ],
      },
      {
        heading: "服务使用",
        paragraphs: [
          "服务按当前功能和可用性提供。您应自行判断 AI 输出是否适合用于决策、生产或对外发布；除非另有明确书面约定，AI 输出不构成专业建议。",
        ],
      },
      {
        heading: "费用与服务调整",
        paragraphs: [
          "涉及充值、订阅、计量或提现的服务，以订单页面、价格页面和适用的专项规则为准。平台可基于安全、合规、运维或产品调整需要修改、暂停或终止部分功能，并会在合理范围内提供通知。",
        ],
      },
      {
        heading: "责任与条款更新",
        paragraphs: [
          "您对使用服务、提交内容及向终端用户提供服务的行为负责。若本条款发生实质变更，平台可在后续登录或继续使用前要求重新确认；仅限格式或非实质性说明的调整不一定触发重新确认。",
        ],
      },
    ],
  },
  {
    id: "cookies",
    title: "Cookie 说明",
    summary: "说明跨子域单点登录所需 Cookie 的用途与控制方式。",
    version: DOCUMENT_VERSION,
    effectiveDate: "2026-07-19",
    sections: [
      {
        heading: "必要 Cookie",
        paragraphs: [
          "平台使用必要的会话 Cookie 支持管理员、租户和终端用户在各自门户中的登录、授权回跳和登出。该 Cookie 采用 HttpOnly、Secure（生产环境）和 SameSite 等安全属性，并按会话配置自动失效。",
        ],
      },
      {
        heading: "非必要技术",
        paragraphs: [
          "当前版本不应在未告知的情况下启用广告或跨站追踪技术。若后续引入非必要的统计、偏好或营销 Cookie，平台应先更新本说明，并按适用要求提供选择或管理入口。",
        ],
      },
      {
        heading: "如何控制",
        paragraphs: [
          "您可以通过浏览器删除或阻止 Cookie，但这样可能导致登录、授权回跳或其他必要功能不可用。",
        ],
      },
    ],
  },
  {
    id: "acceptable-use",
    title: "可接受使用政策",
    summary: "说明 AI、代理和账户服务不得被用于的高风险或滥用行为。",
    version: DOCUMENT_VERSION,
    effectiveDate: "2026-07-19",
    sections: [
      {
        heading: "禁止行为",
        paragraphs: [
          "您不得利用服务实施、协助或规避违法、侵权、欺诈、攻击或滥用行为。",
        ],
        items: [
          "绕过身份验证、速率限制、计费、风控或其他安全措施。",
          "提交、生成、传播或自动化处理违法、侵权、恶意、欺骗性或不具备合法处理授权的内容。",
          "利用 API、模型、代理或账户资源进行未经授权的扫描、入侵、骚扰、垃圾信息或高风险自动化决策。",
          "转让、共享或暴露账户凭据、访问令牌、密钥或其他受保护资源。",
        ],
      },
      {
        heading: "处置",
        paragraphs: [
          "平台可基于安全、滥用防护、上游服务商要求或适用法律，对相关请求、密钥、账户或租户采取限制、暂停、审计或终止措施，并在合理可行的情况下提供说明。",
        ],
      },
    ],
  },
] as const;

export const legalFooterDocumentIDs: readonly LegalDocumentID[] = [
  "privacy",
  "terms",
  "cookies",
  "acceptable-use",
];

export function findLegalDocument(
  id: string | undefined,
): LegalDocument | undefined {
  return legalDocuments.find((document) => document.id === id);
}

export function legalDocumentURL(baseUrl: string, id: LegalDocumentID): string {
  return `${baseUrl.replace(/\/+$/, "")}/${id}`;
}
