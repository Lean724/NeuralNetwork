declare module 'three-spritetext' {
  import * as THREE from 'three';

  export default class SpriteText extends THREE.Sprite {
    constructor(text?: string, textHeight?: number, color?: string);
    text: string;
    textHeight: number;
    color: string;
    backgroundColor: string | boolean;
    borderColor: string;
    borderWidth: number;
    borderRadius: number;
    padding: number | [number, number];
    fontFace: string;
    fontSize: number;
    fontWeight: string;
  }
}
