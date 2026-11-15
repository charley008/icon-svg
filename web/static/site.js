'use strict';
const $=s=>document.querySelector(s),all=s=>document.querySelectorAll(s);
function readStorage(storage,key,fallback){try{return JSON.parse(storage.getItem(key))??fallback;}catch{return fallback;}}
function writeStorage(storage,key,value){try{storage.setItem(key,JSON.stringify(value));}catch{}}
$('#menu-toggle')?.addEventListener('click',()=>{$('#sidebar').classList.toggle('open');$('#scrim').classList.toggle('show');});$('#scrim')?.addEventListener('click',()=>{$('#sidebar').classList.remove('open');$('#scrim').classList.remove('show');});
let toastTimer;function toast(message){const el=$('#toast');if(!el)return;el.textContent=message;el.classList.add('show');clearTimeout(toastTimer);toastTimer=setTimeout(()=>el.classList.remove('show'),3000);}
function previewBackground(value){if(!['soft','white','dark'].includes(value))value='soft';if($('#grid'))$('#grid').dataset.preview=value;all('[data-card-bg]').forEach(b=>b.classList.toggle('active',b.dataset.cardBg===value));try{localStorage.setItem('icon-svg-preview',value);}catch{}}let preview='soft';try{preview=localStorage.getItem('icon-svg-preview')||'soft';}catch{}previewBackground(preview);all('[data-card-bg]').forEach(b=>b.addEventListener('click',()=>previewBackground(b.dataset.cardBg)));
const savedFavorites=readStorage(localStorage,'icon-svg-favorites',[]),favoriteIds=new Set(Array.isArray(savedFavorites)?savedFavorites:[]);
all('[data-favorite-aliases]').forEach(b=>{let moved=false;for(const id of b.dataset.favoriteAliases.split(',')){if(id&&favoriteIds.delete(id))moved=true;}if(moved)favoriteIds.add(b.dataset.favorite);});writeStorage(localStorage,'icon-svg-favorites',[...favoriteIds]);
function updateFavorites(){if($('#favorite-count'))$('#favorite-count').textContent=favoriteIds.size;all('[data-favorite]').forEach(b=>{const active=favoriteIds.has(b.dataset.favorite);b.textContent=active?'♥':'♡';b.classList.toggle('hearted',active);b.setAttribute('aria-pressed',active);});}
all('[data-favorite]').forEach(b=>b.addEventListener('click',()=>{const key=b.dataset.favorite;favoriteIds.has(key)?favoriteIds.delete(key):favoriteIds.add(key);writeStorage(localStorage,'icon-svg-favorites',[...favoriteIds]);updateFavorites();}));updateFavorites();
$('#favorites-nav')?.addEventListener('click',()=>{location.href='/?'+new URLSearchParams({favorites:[...favoriteIds].join(',')});});
const savedSelection=readStorage(sessionStorage,'icon-svg-selection',[]),selectedIds=new Set(Array.isArray(savedSelection)?savedSelection:[]);
function updateSelection(){all('[data-select]').forEach(b=>{const active=selectedIds.has(b.dataset.select);b.textContent=active?'✓':'';b.classList.toggle('selected',active);b.setAttribute('aria-pressed',active);});if($('#selection-tray')){$('#selection-tray').classList.toggle('hidden',!selectedIds.size);$('#selection-count').textContent=selectedIds.size;$('#selected-ids').value=[...selectedIds].join(',');}writeStorage(sessionStorage,'icon-svg-selection',[...selectedIds]);}
all('[data-select]').forEach(b=>b.addEventListener('click',()=>{const id=b.dataset.select;if(selectedIds.has(id))selectedIds.delete(id);else{if(selectedIds.size>=50){toast('一次最多选择 50 个图标');return;}selectedIds.add(id);}updateSelection();}));$('#selection-clear')?.addEventListener('click',()=>{selectedIds.clear();updateSelection();});updateSelection();
document.addEventListener('keydown',e=>{if(e.key==='/'&&!['INPUT','TEXTAREA','SELECT'].includes(document.activeElement.tagName)&&$('#search')){e.preventDefault();$('#search').focus();}});
all('#search-form select').forEach(el=>el.addEventListener('change',()=>$('#search-form').requestSubmit()));
all('form[data-confirm]').forEach(form=>form.addEventListener('submit',e=>{if(!confirm(form.dataset.confirm))e.preventDefault();}));
all('[data-version]').forEach(button=>button.addEventListener('click',()=>{all('[data-version]').forEach(b=>b.classList.toggle('active',b===button));$('#detail-image').src=button.dataset.src;$('#detail-download').href='/download/'+button.dataset.version;$('#version-filename').textContent=button.dataset.file;$('#icon-source').textContent=button.dataset.source;$('#icon-license').textContent=button.dataset.license;}));
$('#preview-size')?.addEventListener('input',e=>{const im=$('#detail-image');if(im){im.style.width=e.target.value+'px';im.style.height=e.target.value+'px';}});all('[data-canvas]').forEach(b=>b.addEventListener('click',()=>$('#detail-canvas').className='detail-canvas '+b.dataset.canvas));
$('#copy-svg')?.addEventListener('click',async()=>{try{const response=await fetch($('#detail-image').src);if(!response.ok)throw new Error('图标已不可用');const content=await response.text();if(navigator.clipboard&&window.isSecureContext)await navigator.clipboard.writeText(content);else{const area=document.createElement('textarea');area.value=content;document.body.append(area);area.select();const copied=document.execCommand('copy');area.remove();if(!copied)throw new Error('浏览器不允许复制，请下载 SVG');}toast('SVG 源码已复制');}catch(e){toast(e.message);}});
$('#download-png')?.addEventListener('click',async()=>{
  const button=$('#download-png'),size=Number($('#png-size').value);
  if(![64,128,256,512,1024,2048].includes(size)){toast('请选择 PNG 尺寸');return;}
  const source=$('#detail-image').src,filename=$('#version-filename').textContent;
  button.disabled=true;
  try{
    const image=new Image();image.src=source;await image.decode();
    if(!image.naturalWidth||!image.naturalHeight)throw new Error('无法读取图标尺寸');
    const canvas=document.createElement('canvas');canvas.width=canvas.height=size;
    const scale=Math.min(size/image.naturalWidth,size/image.naturalHeight);
    const width=image.naturalWidth*scale,height=image.naturalHeight*scale;
    canvas.getContext('2d').drawImage(image,(size-width)/2,(size-height)/2,width,height);
    const blob=await new Promise(resolve=>canvas.toBlob(resolve,'image/png'));
    if(!blob)throw new Error('PNG 导出失败');
    const url=URL.createObjectURL(blob),a=document.createElement('a');a.href=url;
    a.download=filename.replace(/\.svg$/i,'')+'-'+size+'x'+size+'.png';a.click();
    setTimeout(()=>URL.revokeObjectURL(url),1000);
  }catch(e){toast(e.message||'PNG 导出失败');}finally{button.disabled=false;}
});

let brandSearchTimer,brandSearchRequest;
$('#brand-nav-search')?.addEventListener('input',e=>{
  clearTimeout(brandSearchTimer);brandSearchRequest?.abort();
  const query=e.target.value.trim(),controller=new AbortController();brandSearchRequest=controller;
  $('#brand-nav-more').href='/?'+new URLSearchParams({view:'brands',q:query});
  brandSearchTimer=setTimeout(async()=>{
    try{
      const response=await fetch('/api/brands?'+new URLSearchParams({q:query}),{signal:controller.signal});
      if(!response.ok)throw new Error('搜索暂时不可用');
      const brands=await response.json();if(controller.signal.aborted)return;
      all('#categories .brand-nav-item').forEach(link=>link.remove());
      const fragment=document.createDocumentFragment();
      for(const brand of brands){
        const link=document.createElement('a');link.className='nav-item category-button brand-nav-item';link.href='/?brand='+encodeURIComponent(brand.slug);
        if(new URLSearchParams(location.search).get('brand')===brand.slug)link.classList.add('active');
        if(brand.logo){const image=document.createElement('img');image.src=brand.logo;image.width=image.height=22;image.alt='';image.loading='lazy';link.append(image);}
        const label=document.createElement('span');label.className='brand-nav-name';label.textContent=brand.title;link.append(label);fragment.append(link);
      }
      $('#brand-nav-empty').before(fragment);$('#brand-nav-empty').classList.toggle('hidden',brands.length>0);
    }catch(error){if(error.name!=='AbortError')toast('搜索暂时不可用，可按 Enter 查看合集总览');}
  },250);
});

$('#categories .brand-nav-item.active')?.scrollIntoView({block:'nearest'});

function bindAdminDialog(dialogSelector,triggerSelector,closeSelector){
  const dialog=$(dialogSelector);
  $(triggerSelector)?.addEventListener('click',()=>{if(dialog&&!dialog.open)dialog.showModal();});
  all(closeSelector).forEach(button=>button.addEventListener('click',()=>dialog?.close()));
  dialog?.addEventListener('click',event=>{
    if(event.target!==dialog)return;
    const box=dialog.getBoundingClientRect();
    if(event.clientX<box.left||event.clientX>box.right||event.clientY<box.top||event.clientY>box.bottom)dialog.close();
  });
}
bindAdminDialog('#add-brand-dialog','#open-add-brand','[data-close-brand-dialog]');
bindAdminDialog('#add-product-dialog','#open-add-product','[data-close-product-dialog]');

const passwordForm=$('#password-form');
if(passwordForm){
  const password=passwordForm.elements.namedItem('password');
  const confirmation=passwordForm.elements.namedItem('password_confirm');
  const validateConfirmation=()=>confirmation.setCustomValidity(confirmation.value&&confirmation.value!==password.value?'两次输入的新密码不一致':'');
  password.addEventListener('input',validateConfirmation);
  confirmation.addEventListener('input',validateConfirmation);
  passwordForm.addEventListener('submit',event=>{
    validateConfirmation();
    if(!passwordForm.reportValidity())event.preventDefault();
  });
}

const libraryImportForm=document.getElementById('library-import-form');
if(libraryImportForm){
  const button=libraryImportForm.querySelector('[data-import-submit]');
  const progress=document.getElementById('library-import-progress');
  const label=button.textContent;
  libraryImportForm.addEventListener('submit',()=>{button.disabled=true;button.textContent='正在导入…';progress.classList.remove('hidden');});
  window.addEventListener('pageshow',()=>{button.disabled=false;button.textContent=label;progress.classList.add('hidden');});
}

const navigationForm=document.getElementById('navigation-form');
if(navigationForm){
  const rows=document.getElementById('navigation-rows');
  const add=document.getElementById('navigation-add');
  const template=document.getElementById('navigation-row-template');
  const update=()=>{
    const items=Array.from(rows.children);
    add.disabled=items.length>=12;
    document.getElementById('navigation-count').textContent=`${items.length} / 12 条链接，保存后生效。`;
    items.forEach((row,index)=>{
      row.querySelector('legend').textContent=`链接 ${index+1}`;
      row.querySelector('[data-navigation="up"]').disabled=index===0;
      row.querySelector('[data-navigation="down"]').disabled=index===items.length-1;
    });
  };
  add.addEventListener('click',()=>{
    if(rows.children.length>=12)return;
    const row=template.content.firstElementChild.cloneNode(true);
    rows.append(row);
    update();
    row.querySelector('input').focus();
  });
  rows.addEventListener('click',event=>{
    const button=event.target.closest('[data-navigation]');
    if(!button)return;
    const row=button.closest('.navigation-row');
    const action=button.dataset.navigation;
    if(action==='up'&&row.previousElementSibling)rows.insertBefore(row,row.previousElementSibling);
    if(action==='down'&&row.nextElementSibling)rows.insertBefore(row.nextElementSibling,row);
    if(action==='remove'){
      const target=row.nextElementSibling||row.previousElementSibling;
      row.remove();
      (target?target.querySelector('input'):add).focus();
    }
    update();
  });
  update();
}
