import { Node, mergeAttributes } from "@tiptap/core";

export const SignatureBlock = Node.create({
  name: "signatureBlock",
  group: "block",
  content: "block+",
  defining: true,
  parseHTML() {
    return [{ tag: "div[data-mailez-signature]" }];
  },
  addAttributes() {
    return {
      signatureId: {
        default: null,
        parseHTML: (element: HTMLElement) => element.getAttribute("data-mailez-signature"),
        renderHTML: (attributes: Record<string, unknown>) =>
          attributes.signatureId
            ? {"data-mailez-signature": String(attributes.signatureId)}
            : {},
      },
    };
  },
  renderHTML({HTMLAttributes}) {
    return ["div", mergeAttributes(HTMLAttributes), 0];
  },
});
